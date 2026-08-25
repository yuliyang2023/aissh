package sshwrap

import (
	"reflect"
	"testing"
)

func TestParseConnectionAndCommand(t *testing.T) {
	parsed, err := Parse([]string{"-p", "2222", "-J", "jump", "root@server", "uptime"})
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Target != "root@server" {
		t.Fatalf("target = %q", parsed.Target)
	}
	if !reflect.DeepEqual(parsed.ConnectionArgs, []string{"-p", "2222", "-J", "jump"}) {
		t.Fatalf("connection args = %#v", parsed.ConnectionArgs)
	}
	if !reflect.DeepEqual(parsed.RemoteCommand, []string{"uptime"}) {
		t.Fatalf("remote command = %#v", parsed.RemoteCommand)
	}
}

func TestParseAttachedOption(t *testing.T) {
	parsed, err := Parse([]string{"-p2222", "server"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(parsed.MonitorArgs(), []string{"-p2222", "server"}) {
		t.Fatalf("monitor args = %#v", parsed.MonitorArgs())
	}
}

func TestParseRequiresTarget(t *testing.T) {
	if _, err := Parse([]string{"-p", "22"}); err == nil {
		t.Fatal("expected missing target error")
	}
}

func TestHasControlPath(t *testing.T) {
	if !HasControlPath([]string{"-o", "ControlPath=/tmp/ssh", "server"}) {
		t.Fatal("expected ControlPath to be detected")
	}
	if HasControlPath([]string{"-p", "22", "server"}) {
		t.Fatal("unexpected ControlPath")
	}
}
