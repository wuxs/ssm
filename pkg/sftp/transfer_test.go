package sftp

import (
	"errors"
	"reflect"
	"testing"
)

func TestNormalizeRemoteTransferPathsExpandsRemoteSourceForDownload(t *testing.T) {
	tm := &TransferManager{
		remoteCommand: func(command string) ([]byte, error) {
			return []byte("/home/deploy\n"), nil
		},
	}

	source, destination := tm.normalizeRemoteTransferPaths("~/file.txt", "./file.txt", true)

	if source != "/home/deploy/file.txt" {
		t.Fatalf("source = %q, want %q", source, "/home/deploy/file.txt")
	}
	if destination != "./file.txt" {
		t.Fatalf("destination = %q, want %q", destination, "./file.txt")
	}
}

func TestNormalizeRemoteTransferPathsExpandsRemoteDestinationForUpload(t *testing.T) {
	tm := &TransferManager{
		remoteCommand: func(command string) ([]byte, error) {
			return []byte("/home/deploy\n"), nil
		},
	}

	source, destination := tm.normalizeRemoteTransferPaths("local.txt", "~/dest/local.txt", false)

	if source != "local.txt" {
		t.Fatalf("source = %q, want %q", source, "local.txt")
	}
	if destination != "/home/deploy/dest/local.txt" {
		t.Fatalf("destination = %q, want %q", destination, "/home/deploy/dest/local.txt")
	}
}

func TestNormalizeRemoteTransferPathsExpandsRecursiveUploadDestination(t *testing.T) {
	tm := &TransferManager{
		remoteCommand: func(command string) ([]byte, error) {
			return []byte("/home/deploy\n"), nil
		},
	}

	_, destination := tm.normalizeRemoteTransferPaths("dir", "~/dest", false)

	if destination != "/home/deploy/dest" {
		t.Fatalf("destination = %q, want %q", destination, "/home/deploy/dest")
	}
}

func TestGetRemoteHomeDirUsesFallbackCommandAfterUnusableHome(t *testing.T) {
	var commands []string
	tm := &TransferManager{
		remoteCommand: func(command string) ([]byte, error) {
			commands = append(commands, command)
			switch command {
			case "echo $HOME":
				return []byte("$HOME\n"), nil
			case "whoami":
				return []byte("deploy\n"), nil
			default:
				return nil, errors.New("unexpected command")
			}
		},
	}

	homeDir := tm.getRemoteHomeDir()

	if homeDir != "/home/deploy" {
		t.Fatalf("homeDir = %q, want %q", homeDir, "/home/deploy")
	}
	wantCommands := []string{"echo $HOME", "whoami"}
	if !reflect.DeepEqual(commands, wantCommands) {
		t.Fatalf("commands = %v, want %v", commands, wantCommands)
	}
}

func TestGetRemoteHomeDirUsesRootFallback(t *testing.T) {
	tm := &TransferManager{
		remoteCommand: func(command string) ([]byte, error) {
			switch command {
			case "echo $HOME":
				return nil, errors.New("home unavailable")
			case "whoami":
				return []byte("root\n"), nil
			default:
				return nil, errors.New("unexpected command")
			}
		},
	}

	homeDir := tm.getRemoteHomeDir()

	if homeDir != "/root" {
		t.Fatalf("homeDir = %q, want %q", homeDir, "/root")
	}
}
