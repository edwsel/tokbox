package tokbox

//Adapted from https://github.com/cioc/tokbox

import (
	"log"
	"testing"
)

const key = "<your api key here>"
const secret = "<your partner secret here>"

func TestToken(t *testing.T) {
	tokbox := New(key, secret)
	session, err := tokbox.NewSession("", P2P, ArchiveManual)
	if err != nil {
		log.Fatal(err)
		t.FailNow()
	}
	log.Println(session)
	token, err := session.Token(Publisher, "", Hours24)
	if err != nil {
		log.Fatal(err)
		t.FailNow()
	}
	log.Println(token)
}

func TestArchiveLayout(t *testing.T) {
	tokbox := New(key, secret)

	session, err := tokbox.NewSession("", MediaRouter, ArchiveManual)
	if err != nil {
		log.Fatalf("TestArchiveLayout: %s\n", err)
		t.FailNow()
	}

	layout := ArchiveLayout{
		Type:       Custom,
		Stylesheet: "stream.instructor {position: absolute; width: 100%; height: 50%;}",
	}

	err = tokbox.StartArchive(session.SessionId, "archive_name", Composed, layout)
	if err != nil {
		log.Fatalf("TestArchiveLayout: %s\n", err)
		t.FailNow()
	}
}
