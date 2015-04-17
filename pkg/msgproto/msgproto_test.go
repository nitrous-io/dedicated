package msgproto

import (
	"bytes"
	"io"
	"testing"
)

type fakeWebSocket struct {
	messages [][]byte
	currMsg  int
}

func (f *fakeWebSocket) ReadMessage() (int, []byte, error) {
	if f.currMsg >= len(f.messages) {
		return 0, nil, io.EOF
	}
	d := f.messages[f.currMsg]
	f.currMsg++
	return 0, d, nil
}

func strArrToBytesArr(a []string) [][]byte {
	n := len(a)
	res := make([][]byte, n, n)
	for i, s := range a {
		res[i] = []byte(s)
	}
	return res
}

func TestReadAndDecodeMessage(t *testing.T) {
	msgs := []string{"d|t", "d|hey", "d|d|", "c|lola"}
	exp := []string{"t", "hey", "d|", "lola"}
	fake := &fakeWebSocket{
		messages: strArrToBytesArr(msgs),
	}
	got := make([]string, 0)
	for {
		msgType, d, err := ReadAndDecodeMessage(fake)
		if err != nil {
			if err != io.EOF {
				t.Errorf("ReadAndDecodeMessage() returned err other than io.EOF: %s", err)
				return
			}
			break
		}
		got = append(got, string(d))
		n := len(got)
		switch msgType {
		case DataMessage:
			if n > 3 {
				t.Errorf("got DataMessage from non-data packet '%s'", msgs[n-1])
				return
			}
		case CloseMessage:
			if n != 4 {
				t.Errorf("got CloseMessage from non-close packet '%s'", msgs[n-1])
				return
			}
		default:
			t.Errorf("unknown message type %d", msgType)
			return
		}
	}
	for i, _ := range exp {
		if exp[i] != got[i] {
			t.Errorf("exp[%d]='%s' != got[%d]='%s'", i, exp[i], i, got[i])
		}
	}
}

func TestWriteDataMessage(t *testing.T) {
	data := []string{"", "s1", "d", "asdfasfd", "d|"}
	var expBuf bytes.Buffer
	var gotBuf bytes.Buffer
	for _, s := range data {
		expBuf.Write([]byte("d|"))
		expBuf.Write([]byte(s))
		_, err := WriteDataMessage([]byte(s), &gotBuf)
		if err != nil {
			t.Errorf("WriteDataMessage() failed with %s", err)
			return
		}
	}
	exp := string(expBuf.Bytes())
	got := string(gotBuf.Bytes())
	if exp != got {
		t.Errorf("'%s' != '%s'", exp, got)
	}
}
