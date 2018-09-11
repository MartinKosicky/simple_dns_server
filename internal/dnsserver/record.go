package dnsserver

import (
	"bytes"
	"encoding/binary"
	"net"
)

type Question interface {
	Id() uint16
	QName() string
}

type DnsError interface {
	Error() string
	Code() int32
}

const (
	NotAQuestion           = iota
	NotAStandardQuery      = iota
	TruncationNotSupported = iota
	NoQuestions            = iota
	MultipleQuestions      = iota
	FailedParsingBuffer    = iota
	QClassNotInet          = iota
	QTypeNotAddress        = iota
)

type dnsError struct {
	code int32
	e    string
}

func (e dnsError) Error() string {
	return e.e
}

func (e dnsError) Code() int32 {
	return e.code
}

type question struct {
	id           uint16
	qname        string
	question     []byte
	questionName []byte
}

func (e question) Id() uint16 {
	return e.id
}

func (e question) QName() string {
	return e.qname
}

func makeError(code int32, msg string) dnsError {
	return dnsError{code: code, e: msg}
}

func ParseBuffer(buffer []byte) (Question, DnsError) {

	bufferLength := len(buffer)

	if bufferLength < 12 {
		return nil, makeError(FailedParsingBuffer, "Buffer too small")
	}

	reader := bytes.NewReader(buffer)
	var header struct {
		Id          uint16
		SecondLine1 uint8
		SecondLine2 uint8
		QdCount     uint16
		AnCount     uint16
		NsCount     uint16
		ArCount     uint16
	}
	err := binary.Read(reader, binary.BigEndian, &header)
	if err != nil {
		return nil, makeError(FailedParsingBuffer, err.Error())
	}

	QR := header.SecondLine1 >> 7
	if QR != 0 {
		return nil, makeError(NotAQuestion, "Request is not a query")
	}

	Opcode := (header.SecondLine1 >> 3) & 0xF
	if Opcode != 0 {
		return nil, makeError(NotAStandardQuery, "Request is not  a standard query")
	}

	Truncation := (header.SecondLine1 >> 1) & 1
	if Truncation != 0 {
		return nil, makeError(TruncationNotSupported, "Request is truncated but its not supported")
	}

	if header.QdCount == 0 {
		return nil, makeError(NoQuestions, "Request has no questions")
	}

	if header.QdCount > 1 {
		return nil, makeError(MultipleQuestions, "Request has multiple questions")
	}

	if len(buffer) < 13 {
		return nil, makeError(FailedParsingBuffer, "Buffer too small #1")
	}

	var qnameBuffer bytes.Buffer

	bufferPos := 12
	questionStart := bufferPos

	qNameLength := int(buffer[bufferPos])

	for qNameLength != 0 {
		bufferPos = bufferPos + 1

		if (bufferPos + qNameLength) >= bufferLength {
			return nil, makeError(FailedParsingBuffer, "Buffer too small #2")
		}

		if qnameBuffer.Len() > 0 {
			qnameBuffer.WriteString(".")
		}
		qnameBuffer.Write(buffer[bufferPos : bufferPos+qNameLength])

		bufferPos = bufferPos + qNameLength
		qNameLength = int(buffer[bufferPos])
	}

	bufferPos = bufferPos + 1
	questionNameEnd := bufferPos

	if bufferPos+4 > bufferLength {
		return nil, makeError(FailedParsingBuffer, "Buffer too small #3")
	}

	qType := binary.BigEndian.Uint16(buffer[bufferPos : bufferPos+2])
	bufferPos = bufferPos + 2
	qClass := binary.BigEndian.Uint16(buffer[bufferPos : bufferPos+2])
	bufferPos = bufferPos + 2
	questionEnd := bufferPos

	if qClass != 1 {
		return nil, makeError(QClassNotInet, "QClass is not an IN")
	}

	if qType != 1 {
		return nil, makeError(QTypeNotAddress, "QType not a host adddress (A)")
	}

	return question{id: header.Id, qname: qnameBuffer.String(), question: buffer[questionStart:questionEnd],
		questionName: buffer[questionStart:questionNameEnd]}, nil
}

func MakeResponse(q Question, resultAddress string) []byte {

	qq := q.(question)

	var header struct {
		Id          uint16
		SecondLine1 uint8
		SecondLine2 uint8
		QdCount     uint16
		AnCount     uint16
		NsCount     uint16
		ArCount     uint16
	}

	header.Id = qq.id
	header.SecondLine1 = (1 << 7)
	header.SecondLine2 = 0
	header.QdCount = 1
	header.AnCount = 1
	header.NsCount = 0
	header.ArCount = 0

	var msgBuffer bytes.Buffer

	binary.Write(&msgBuffer, binary.BigEndian, header)
	binary.Write(&msgBuffer, binary.BigEndian, qq.question)
	binary.Write(&msgBuffer, binary.BigEndian, qq.questionName)

	var resourceRecord struct {
		Type     uint16
		Class    uint16
		TTL      uint32
		RDLength uint16
		Addr     [4]byte
	}

	resourceRecord.Type = 1
	resourceRecord.Class = 1
	resourceRecord.TTL = 60
	resourceRecord.RDLength = 4

	addrIP := net.ParseIP(resultAddress).To4()
	resourceRecord.Addr = [4]byte{addrIP[0], addrIP[1], addrIP[2], addrIP[3]}

	binary.Write(&msgBuffer, binary.BigEndian, resourceRecord)
	return msgBuffer.Bytes()
}

func MakeEmptyResponse(q Question) []byte {
	qq := q.(question)

	var header struct {
		Id          uint16
		SecondLine1 uint8
		SecondLine2 uint8
		QdCount     uint16
		AnCount     uint16
		NsCount     uint16
		ArCount     uint16
	}

	header.Id = qq.id
	header.SecondLine1 = (1 << 7)
	header.SecondLine2 = 0
	header.QdCount = 1
	header.AnCount = 0
	header.NsCount = 0
	header.ArCount = 0

	var msgBuffer bytes.Buffer

	binary.Write(&msgBuffer, binary.BigEndian, header)
	binary.Write(&msgBuffer, binary.BigEndian, qq.question)

	return msgBuffer.Bytes()
}
