
package structex

import (
	"fmt"
	"io"
	"math"
	"reflect"
	"encoding"
	"encoding/binary"
	"sync"
)

type encoder struct {
	bitWriter*   bitstream.BitWriter
	currentByte uint8
	byteOffset  uint64
	bitOffset   uint64
}

//This Encoder allows the transmission of type and data information to the
// other side of a connection
type Encoder struct {
	mutex      sync.Mutex              // each item must be sent atomically
	w          []io.Writer             // where to send the data
	sent       map[reflect.Type]typeId // which types we've already sent
	countState *encoderState           // stage for writing counts
	freeList   *encoderState           // avoids reallocation
	byteBuf    encoderBuffer               // buffer
	err        error
	ie         Nas5GSUpdateType
}


const maxLength = 9 // Maximum size of an encoded struct
var spaceForLength = make([]byte, maxLength)

// NewEncoder returns a new encoder that will transmit on the io.Writer.
func NewEncoder(w io.Writer) *Encoder {
	enc := new(Encoder)
	enc.w = []io.Writer{w}
	enc.sent = make(map[reflect.Type]typeId)
	enc.countState = enc.newEncoderState(new(encoderBuffer))
	return enc
}


// writeMessage sends the data item preceded by a unsigned count of its length.
func (enc *Encoder) writeMessage(w io.Writer, b *encoderBuffer) {
	//grab the slice from the bytes.Buffer and massage it 
	message := b.Bytes()
	messageLen := len(message) - maxLength

	if messageLen >= tooBig {
		enc.setError(errors.New("gob: encoder: message too big"))
		return
	}
	// Encode the length
	enc.countState.b.Reset()
	enc.countState.encodeUint(uint64(messageLen))
	// Copy the length to be a prefix of the message
	offset := maxLength - enc.countState.b.Len()
	copy(message[offset:], enc.countState.b.Bytes())
	// Write the data
	_, err := w.Write(message[offset:])
	// Drain the buffer and restore the space at the front for the count of the next message
	b.Reset()
	b.Write(spaceForLength)
	if err != nil {
		enc.setError(err)
	}
}


func (enc *Encoder) sendActualType(w io.Writer, state *encoderState, ut *userTypeInfo, actual reflect.Type) (sent bool) {
	if _, alreadySent := enc.sent[actual]; alreadySent {
		return false
	}
	info, err := getTypeInfo(ut)
	if err != nil {
		enc.setError(err)
		return
	}
	

	state.encodeInt(-int64(info.id))
	enc.encode(state.b, reflect.ValueOf(info.wire), wireTypeUserInfo)
	enc.writeMessage(w, state.b)
	if enc.err != nil {
		return
	}

	
	enc.sent[ut.base] = info.id
	if ut.user != ut.base {
		enc.sent[ut.user] = info.id
	}
	
	switch st := actual; st.Kind() {
	case reflect.Struct:
		for i := 0; i < st.NumField(); i++ {
			if isExported(st.Field(i).Name) {
				enc.sendType(w, state, st.Field(i).Type)
			}
		}
	case reflect.Array, reflect.Slice:
		enc.sendType(w, state, st.Elem())
	case reflect.Map:
		enc.sendType(w, state, st.Key())
		enc.sendType(w, state, st.Elem())
	}
	return true
}

//If it is needed sends the type info to the other side
func (enc *Encoder) sendType(w io.Writer, state *encoderState, origt reflect.Type) (sent bool) {
	ut := userType(origt)
	if ut.externalEnc != 0 {
		return enc.sendActualType(w, state, ut, ut.base)
	}


	switch rt := ut.base; rt.Kind() {
	default:
		return
	case reflect.Slice:
		if rt.Elem().Kind() == reflect.Uint8 {
			return
		}
		break
	case reflect.Array:
		break
	case reflect.Map:
		break
	case reflect.Struct:
		break
	case reflect.Chan, reflect.Func:
		return
	}

	return enc.sendActualType(w, state, ut, ut.base)
}


func (enc *Encoder) Encode(e interface{}) error {
	return enc.EncodeValue(reflect.ValueOf(e))
}

//We need this function if it's first time the type is sent
func (enc *Encoder) sendTypeDescriptor(w io.Writer, state *encoderState, ut *userTypeInfo) {
	
	rt := ut.base
	if ut.externalEnc != 0 {
		rt = ut.user
	}
	if _, alreadySent := enc.sent[rt]; !alreadySent {
		ent := enc.sendType(w, state, rt)
		if enc.err != nil {
			return
		}
		
		if !sent {
			info, err := getTypeInfo(ut)
			if err != nil {
				enc.setError(err)
				return
			}
			enc.sent[rt] = info.id
		}
	}
}


func (enc *Encoder) sendTypeId(state *encoderState, ut *userTypeInfo) {
	state.encodeInt(int64(enc.sent[ut.base]))
}


func (ie Nas5GSUpdateType) Encode(buffer *bytes.Buffer) error {
	if buffer.Kind() == reflect.Invalid {
		return errors.New("Can not encode null value")
	}
	if buffer.Kind() == reflect.Ptr && value.IsNil() {
		panic("Can not encode null pointer")
	}

	enc.mutex.Lock()
	defer enc.mutex.Unlock()


	enc.w = enc.w[0:1]

	ut, err := validUserType(value.Type())
	if err != nil {
		return err
	}

	enc.err = nil
	enc.byteBuf.Reset()
	enc.byteBuf.Write(spaceForLength)
	state := enc.newEncoderState(&enc.byteBuf)

	enc.sendTypeDescriptor(enc.writer(), state, ut)
	enc.sendTypeId(state, ut)
	if enc.err != nil {
		return enc.err
	}

	// Encode some object
	enc.encode(state.b, value, ut)
	if enc.err == nil {
		enc.writeMessage(enc.writer(), state.b)
	}

	enc.freeEncoderState(state)
	return enc.err
}
