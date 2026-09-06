package chat

import (
	"github.com/google/uuid"
	"strings"
	"testing"
)

func TestSendValidation(t *testing.T) {
	for _, body := range []string{"", "  ", strings.Repeat("a", 5001), string([]byte{0xff})} {
		if (SendInput{ConversationID: uuid.New(), ClientMessageID: uuid.New(), Body: body}).Validate() == nil {
			t.Fatal("invalid body accepted")
		}
	}
	if err := (SendInput{ConversationID: uuid.New(), ClientMessageID: uuid.New(), Body: "Xin chào shop"}).Validate(); err != nil {
		t.Fatal(err)
	}
	if (SendInput{Body: "hello"}).Validate() == nil {
		t.Fatal("missing IDs accepted")
	}
}
