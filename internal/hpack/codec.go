package hpack

type Codec struct {
	Encoder Encoder
	Decoder Decoder
}

func NewCodec() *Codec {
	dt := NewDynamicTable()
	return &Codec{
		Encoder: NewEncoder(dt),
		Decoder: NewDecoder(dt),
	}
}
