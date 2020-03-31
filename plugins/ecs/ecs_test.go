package ecs

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

func TestContainerID(t *testing.T) {
	tmp, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp)

	want := "42e85902377f5b9e758dfa6537377e2da86338b4b40c20d875251082e8a1da84"
	dummyCGroup := filepath.Join(tmp, "tmpfile")
	if err := ioutil.WriteFile(dummyCGroup, []byte("14:name=systemd:/docker/"+want+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	got := containerID(dummyCGroup)
	if got != want {
		t.Errorf("want %s, got %s", want, got)
	}
}
