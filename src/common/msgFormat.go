package common

// MaxPayloadSize ...
// As MSB is 0 for push operation, max payload size would be 127 bytes
const MaxPayloadSize = 127

// MaxRequestSize ...
const MaxRequestSize = MaxPayloadSize + 1

// PushReqData specifies the data type for push operation
//   header having MSB=0 and rest is length of payload
type PushReqData struct {
	Header  byte
	Payload []byte
}

// PushResData specifies the data type for the result of push operation
type PushResData struct {
	Data byte
}

// GetPushReqData ...
func GetPushReqData(data string) *PushReqData {
	pushReqData := &PushReqData{}
	if data != "" {
		payloadLen := len(data)
		if payloadLen <= MaxPayloadSize {
			pushReqData.Header = byte(payloadLen)
			pushReqData.Payload = []byte(data)
		}
	}
	return pushReqData
}

// GetPushResData ...
func GetPushResData() *PushResData {
	return &PushResData{
		Data: 0,
	}
}

// PopReqData ...
type PopReqData struct {
	Header byte
}

// PopResData ...
type PopResData struct {
	Header  byte
	Payload []byte
}

// GetPopReqData ...
func GetPopReqData() *PopReqData {
	// Set MSB to 1 to specify pop request
	return &PopReqData{
		Header: 128, //10000000 is binary for 128
	}
}

// GetPopResData takes data as input and returns popResData
func GetPopResData(data []byte) *PopResData {
	popResponse := &PopResData{}
	payloadLen := len(data)
	if payloadLen == 0 {
		panic("Invalid data size")
	}

	if payloadLen <= MaxPayloadSize {
		popResponse.Header = byte(payloadLen)
		popResponse.Payload = data
	}
	return popResponse
}
