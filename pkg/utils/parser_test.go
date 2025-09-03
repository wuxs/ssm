// pkg/utils/parser.go
package utils

import "testing"

func TestParseSSHHost(t *testing.T) {
	type args struct {
		host string
	}
	tests := []struct {
		name         string
		args         args
		wantUsername string
		wantHostname string
		wantPort     string
	}{
		{
			name:         "Test case 1",
			args:         args{host: "user@host:port"},
			wantUsername: "user",
			wantHostname: "host",
			wantPort:     "port",
		},
		{
			name:         "Test case 1",
			args:         args{host: "user@host"},
			wantUsername: "user",
			wantHostname: "host",
			wantPort:     "22",
		},
		{
			name:         "Test case 1",
			args:         args{host: "host"},
			wantUsername: "wuxs",
			wantHostname: "host",
			wantPort:     "22",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotUsername, gotHostname, gotPort := ParseSSHHost(tt.args.host)
			if gotUsername != tt.wantUsername {
				t.Errorf("ParseSSHHost() gotUsername = %v, want %v", gotUsername, tt.wantUsername)
			}
			if gotHostname != tt.wantHostname {
				t.Errorf("ParseSSHHost() gotHostname = %v, want %v", gotHostname, tt.wantHostname)
			}
			if gotPort != tt.wantPort {
				t.Errorf("ParseSSHHost() gotPort = %v, want %v", gotPort, tt.wantPort)
			}
		})
	}
}
