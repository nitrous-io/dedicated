package collab_test

type MockMessageReaderWriter struct {
	R       [][]byte
	W       [][]byte
	MsgType int
	Err     error
}

func NewMockMessageReaderWriter() *MockMessageReaderWriter {
	return &MockMessageReaderWriter{
		R:       [][]byte{},
		W:       [][]byte{},
		MsgType: 1,
		Err:     nil,
	}
}

func (m *MockMessageReaderWriter) ReadMessage() (int, []byte, error) {
	if m.Err != nil {
		return 0, nil, m.Err
	}
	msg := m.R[0]
	m.R = m.R[1:]
	return m.MsgType, msg, nil
}

func (m *MockMessageReaderWriter) Write(p []byte) (int, error) {
	m.R = append(m.R, p)
	return len(p), nil
}

// TODO: integration test
