package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/wuxs/ssm/pkg/config"
	"github.com/wuxs/ssm/pkg/sshconfig"
)

func TestHistoryCandidates(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".ssh", "config"), []byte("Include saved.conf\nHost *\n  User fallback\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".ssh", "saved.conf"), []byte(`Host prod prod-copy
  HostName 10.0.0.2
  User root
  Port 2222
  # SSM-Password: plain:do-not-output
  # SSM-Remark: do-not-output
  IdentityFile ~/.ssh/prod_key
  ProxyJump gateway
Host 192.168.1.159
  HostName 192.168.1.159
  User root
  Port 2222
Host 192.168.1.227
  HostName 192.168.1.227
  User root
  Port 22
Host bad;command
  HostName bad$host
  User root
`), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		prefix string
		want   []string
	}{
		{"pro", []string{"prod", "prod-copy"}},
		{"root@192.168.1.", []string{"root@192.168.1.159:2222", "root@192.168.1.227:22"}},
		{"does-not-exist", nil},
		{"bad", nil},
		{"root@10.0.0.2", nil},
		{"root@192.168.1.159:22", []string{"root@192.168.1.159:2222"}},
		{"root@192.168.1.159:2222", []string{"root@192.168.1.159:2222"}},
		{"do-not-output", nil},
	} {
		t.Run(tc.prefix, func(t *testing.T) {
			got, err := historyCandidates(tc.prefix)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("candidates = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestHistoryCandidatesMissingOrInvalidConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	got, err := historyCandidates("")
	if err != nil || len(got) != 0 {
		t.Fatalf("missing config: candidates = %q, err = %v", got, err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".ssh", "config"), []byte("Include [invalid\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := historyCandidates(""); err == nil {
		t.Fatal("expected invalid Include error")
	}
}

func TestBashHistoryCompletionState(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("Bash is not installed")
	}
	script := bashHistoryCompletion + `
_ssm_tab_emacs_action=complete
_ssm_tab_emacs_kind=readline
ssm() { printf '%s\n' root@192.168.1.159:22 root@192.168.1.227:22; }
check() {
    if [[ $READLINE_LINE != "$1" || $READLINE_POINT != "$2" ]]; then
        printf 'got line=%q point=%s, want line=%q point=%s\n' "$READLINE_LINE" "$READLINE_POINT" "$1" "$2"
        exit 1
    fi
}
READLINE_LINE='ssm root@192.168. --port 22'
READLINE_POINT=17
_ssm_history_tab emacs-standard 2>/dev/null
check 'ssm root@192.168.1.159:22 --port 22' 17
_ssm_history_tab emacs-standard 2>/dev/null
check 'ssm root@192.168.1.227:22 --port 22' 17
_ssm_history_tab emacs-standard 2>/dev/null
check 'ssm root@192.168.1.159:22 --port 22' 17
# Typing at the fixed cursor replaces the old provisional completion on Tab.
READLINE_LINE='ssm root@192.168.1.1.159:22 --port 22'
READLINE_POINT=19
_ssm_history_tab emacs-standard 2>/dev/null
check 'ssm root@192.168.1.159:22 --port 22' 19
# A new prompt must not treat coincidentally identical real text as provisional.
_ssm_history_prefix='ssm root@192.168.'
_ssm_history_inserted='1.159:22'
_ssm_history_suffix=' --port 22'
_ssm_history_command=-1
READLINE_LINE='ssm root@192.168.1.1.159:22 --port 22'
READLINE_POINT=19
_ssm_history_tab emacs-standard 2>/dev/null
check 'ssm root@192.168.1.159:221.159:22 --port 22' 19
# Changing the prefix starts a new candidate set.
READLINE_LINE='ssm root@192.168.1.2'
READLINE_POINT=${#READLINE_LINE}
_ssm_history_tab emacs-standard 2>/dev/null
check 'ssm root@192.168.1.227:22' 20
# No matches and non-SSM input preserve the line and cursor.
READLINE_LINE='ssm unknown'
READLINE_POINT=11
_ssm_history_tab emacs-standard 2>/dev/null
check 'ssm unknown' 11
READLINE_LINE='cat /tm'
READLINE_POINT=7
_ssm_history_tab emacs-standard 2>/dev/null
check 'cat /tm' 7
# Options with non-host values and subcommands do not get history candidates.
READLINE_LINE='ssm -i root@192.168.'
READLINE_POINT=${#READLINE_LINE}
_ssm_history_tab emacs-standard 2>/dev/null
check 'ssm -i root@192.168.' 20
READLINE_LINE='ssm cp root@192.168.'
READLINE_POINT=${#READLINE_LINE}
_ssm_history_tab emacs-standard 2>/dev/null
check 'ssm cp root@192.168.' 20
# A host-valued flag gets the same history completion.
READLINE_LINE='ssm -J root@192.168.'
READLINE_POINT=${#READLINE_LINE}
_ssm_history_tab emacs-standard 2>/dev/null
check 'ssm -J root@192.168.1.159:22' 20
`
	cmd := exec.Command(bash, "--noprofile", "--norc", "-c", script)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Bash completion: %v\n%s", err, output)
	}
}

func TestBashHistoryCompletionPreservesTabBindings(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("Bash is not installed")
	}
	script := bashHistoryCompletion + `
set -e
# Remember a user's Readline action, including across repeated installation.
bind -m emacs-standard '"\C-i": menu-complete'
_ssm_history_bind emacs-standard
_ssm_history_bind emacs-standard
[[ $_ssm_tab_emacs_action == menu-complete ]]
READLINE_LINE='cat /tm'
READLINE_POINT=7
_ssm_history_tab emacs-standard
[[ $(bind -m emacs-standard -p) == *'"\e[99;2~": menu-complete'* ]]
# Also preserve user macros and bind -x shell actions.
unset _ssm_tab_emacs_action _ssm_tab_emacs_kind
bind -m emacs-standard '"\C-i": "ORIGINAL"'
_ssm_history_bind emacs-standard
_ssm_history_tab emacs-standard
[[ $(bind -m emacs-standard -s) == *'"\e[99;2~": "ORIGINAL"'* ]]
unset _ssm_tab_emacs_action _ssm_tab_emacs_kind
bind -m emacs-standard -x '"\C-i": "_custom_action"'
_ssm_history_bind emacs-standard
_ssm_history_tab emacs-standard
[[ $(bind -m emacs-standard -X) == *'"\e[99;2~": "_custom_action"'* ]]
`
	cmd := exec.Command(bash, "--noprofile", "--norc", "-c", script)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Bash Tab bindings: %v\n%s", err, output)
	}
}

func TestHistoryCandidatesPreserveConnectionOptions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfg := &config.SSHConfig{Alias: "192.168.1.159", Host: "192.168.1.159", Username: "root", Port: "2222", PrivateKey: "/tmp/special_key", ProxyJump: "gateway", Password: "fixture-password"}
	if err := sshconfig.SaveHost(cfg); err != nil {
		t.Fatal(err)
	}
	got, err := historyCandidates("root@192.168.")
	if err != nil || !reflect.DeepEqual(got, []string{"root@192.168.1.159:2222"}) {
		t.Fatalf("saved history candidates = %q, err = %v", got, err)
	}
	resolved, err := sshconfig.Resolve(got[0], sshconfig.Options{})
	if err != nil || resolved.PrivateKey != cfg.PrivateKey || resolved.ProxyJump != cfg.ProxyJump || resolved.Password != cfg.Password {
		t.Fatalf("saved target lost options: %#v, err = %v", resolved, err)
	}
	if err := os.WriteFile(filepath.Join(home, ".ssh", "config"), []byte(`Host prod
  HostName 10.0.0.2
  User deploy
  Port 2200
  IdentityFile ~/.ssh/prod_key
  ProxyJump gateway
  # SSM-Password: plain:fixture-password
`), 0600); err != nil {
		t.Fatal(err)
	}
	got, err = historyCandidates("deploy@")
	if err != nil || !reflect.DeepEqual(got, []string{"deploy@prod:2200"}) {
		t.Fatalf("alias candidates = %q, err = %v", got, err)
	}
	resolved, err = sshconfig.Resolve(got[0], sshconfig.Options{})
	if err != nil || resolved.Host != "10.0.0.2" || resolved.PrivateKey != filepath.Join(home, ".ssh", "prod_key") || resolved.ProxyJump != "gateway" || resolved.Password != "fixture-password" {
		t.Fatalf("qualified alias lost options: %#v, err = %v", resolved, err)
	}
}

func TestBashHistoryCompletionNativeMenu(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("Bash is not installed")
	}
	dir := t.TempDir()
	for _, name := range []string{"one-aa.txt", "one-ab.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	scriptPath := filepath.Join(dir, "completion.bash")
	if err := os.WriteFile(scriptPath, []byte(bashHistoryCompletion), 0600); err != nil {
		t.Fatal(err)
	}
	input := `bind '"\C-i": menu-complete'
source "$SSM_COMPLETION_SCRIPT"
source "$SSM_COMPLETION_SCRIPT"
_dump() { printf 'DUMP=<%s>\n' "$READLINE_LINE"; }
bind -x '"\C-g": _dump'
` + "cat one-\t\t\a\x15exit\n"
	cmd := exec.Command(bash, "--noprofile", "--norc", "-i")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "SSM_COMPLETION_SCRIPT="+scriptPath, "HISTFILE=/dev/null", "INPUTRC=/dev/null", "PROMPT_COMMAND=", "PS1=prompt> ")
	cmd.Stdin = strings.NewReader(input)
	output, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(output), "DUMP=<cat one-ab.txt >") {
		t.Fatalf("native menu completion: %v\n%s", err, output)
	}
}

func TestBashHistoryCompletionPromptHook(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("Bash is not installed")
	}
	script := bashHistoryCompletion + `
set -e
PROMPT_COMMAND='echo original; true; # keep trailing comment'
_ssm_history_install_prompt
_ssm_history_install_prompt
[[ $PROMPT_COMMAND == $'echo original; true; # keep trailing comment\n_ssm_history_prompt' ]]
[[ $(declare -p PROMPT_COMMAND) != 'declare -a'* ]]
bash -n <<< "$PROMPT_COMMAND"
PROMPT_COMMAND=('echo first' 'echo second')
_ssm_history_install_prompt
_ssm_history_install_prompt
[[ ${#PROMPT_COMMAND[@]} == 3 && ${PROMPT_COMMAND[0]} == 'echo first' && ${PROMPT_COMMAND[1]} == 'echo second' && ${PROMPT_COMMAND[2]} == _ssm_history_prompt ]]
set +e
(exit 42)
_ssm_history_prompt
[[ $? == 42 ]] || exit 1
`
	cmd := exec.Command(bash, "--noprofile", "--norc", "-c", script)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("prompt hook: %v\n%s", err, output)
	}
}
