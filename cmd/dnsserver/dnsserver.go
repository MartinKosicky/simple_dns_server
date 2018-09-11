package main

import (
	"bufio"
	"bytes"
	"github.com/MartinKosicky/simple_dns_server/internal/dnsserver"

	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

var stateMutex sync.Mutex
var hostMap map[string]string

func handleRequest(con net.PacketConn, addr net.Addr, buffer []byte, waiting *sync.WaitGroup) {

	question, err := dnsserver.ParseBuffer(buffer)

	if err != nil {
		println("Code: ", err.Code(), "   Error:", err.Error())
	} else {
		println("Question: ", question.QName())

		stateMutex.Lock()
		resultIp, ok := hostMap[question.QName()]
		stateMutex.Unlock()
		var result []byte

		if ok {
			println("Answer for ", question.QName(), ": ", resultIp)
			result = dnsserver.MakeResponse(question, resultIp)
		} else {
			println("No aswer for ", question.QName())
			result = dnsserver.MakeEmptyResponse(question)
		}

		con.WriteTo(result, addr)
	}

	_ = question
	_ = err
	waiting.Done()

}

func StateRefresher(waiting *sync.WaitGroup, shutdownChannel chan bool) {

	doShutdown := false

	cmdExtraction := func() {

		cmd := exec.Command(os.Args[1], os.Args[2:]...)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		if err != nil {
			println("Error running command. ", err.Error())
			return
		}

		println("StateRefresher: Going to scan output of format : ip;host")

		scanner := bufio.NewScanner(&stdout)
		newHostMap := make(map[string]string)
		for scanner.Scan() {
			line := scanner.Text()
			parts := strings.SplitN(line, ";", 2)

			if len(parts) == 2 {
				println("StateRefresher: Got record:  ip:", parts[0], "  host:", parts[1])
				newHostMap[parts[1]] = parts[0]
			}
		}

		stateMutex.Lock()
		hostMap = newHostMap
		stateMutex.Unlock()
	}

	cmdExtraction()

	for !doShutdown {

		select {
		case _ = <-shutdownChannel:
			println("StateRefresher: recvd shutdown request")
			doShutdown = true

		case <-time.After(2 * time.Minute):
			cmdExtraction()
		}
	}

	waiting.Done()
	println("StateRefresher: shutdown")
}

func StartListening(waiting *sync.WaitGroup) {

	refreshShutdown := make(chan bool)
	waiting.Add(1)
	go StateRefresher(waiting, refreshShutdown)

	refreshShutdownFunc := func() {
		refreshShutdown <- true
	}

	defer refreshShutdownFunc()
	defer waiting.Done()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	newBuffer := make([]byte, 512)

	for {

		println("Going to listen for udp packets")
		con, err := net.ListenPacket("udp", "0.0.0.0:53")
		defer con.Close()

		if err != nil {
			println("Error listening for udp: ", err.Error())
			println("Retrying in 5 secs")
			<-time.After(time.Second * 5)
			continue
		}

		for {

			buffer := newBuffer

			con.SetReadDeadline(time.Now().Add(time.Second * time.Duration(1)))
			n, addr, err := con.ReadFrom(buffer)

			select {
			case <-sigs:
				return
			default:
			}

			if n > 0 {
				newBuffer = make([]byte, 512)
				println("Got packet")
				waiting.Add(1)
				go handleRequest(con, addr, buffer[0:n], waiting)
			} else if err != nil {

				opError, ok := err.(*net.OpError)
				if ok {
					if opError.Timeout() {
						continue
					}

					if !opError.Temporary() {
						println("Failed to read udp not temporary: ", err.Error())
						println("Retrying in 5 secs")
						<-time.After(time.Second * 5)
						break
					} else {
						println("Failed to read udp, temporary error: ", err.Error())
					}
				} else {
					println("Unknown error, restarting")
				}
			}
		}
	}
	println("Listening: finished")
}

func main() {
	if len(os.Args) < 2 {
		log.Fatal("Usage is ", os.Args[0], " command_to_fetch_records")
	}

	hostMap = make(map[string]string)

	var waiting sync.WaitGroup

	waiting.Add(1)
	go StartListening(&waiting)

	waiting.Wait()
	println("Shutting down")

}
