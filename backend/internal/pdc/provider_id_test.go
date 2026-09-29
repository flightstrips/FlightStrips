package pdc

import "testing"

func TestHoppieMessageCommandIDUsesCompleteProviderFrame(t *testing.T) {
	first, err := HoppieMessageCommandID(Message{Raw: "(from:SAS101 to:EKCH type:cpdlc packet:WILCO)"})
	if err != nil {
		t.Fatal(err)
	}
	retry, err := HoppieMessageCommandID(Message{Raw: "(from:SAS101 to:EKCH type:cpdlc packet:WILCO)"})
	if err != nil || retry != first {
		t.Fatalf("poll retry changed ID: %s %v", retry, err)
	}
	different, _ := HoppieMessageCommandID(Message{Raw: "(from:SAS101 to:EKCH type:cpdlc packet:UNABLE)"})
	if different == first {
		t.Fatal("different provider frames share a command ID")
	}
	if _, err := HoppieMessageCommandID(Message{}); err == nil {
		t.Fatal("missing provider frame accepted")
	}
}
