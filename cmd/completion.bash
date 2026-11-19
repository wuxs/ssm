# Enable with: source <(ssm completion bash)
# READLINE_POINT counts bytes, so use the C locale for line slicing.
_ssm_history_tab() {
    local LC_ALL=C
    local keymap=$1 before token command pending= positional=0 word candidate action kind
    local -a words
    if [[ $keymap == emacs-standard ]]; then
        action=$_ssm_tab_emacs_action kind=$_ssm_tab_emacs_kind
    else
        action=$_ssm_tab_vi_action kind=$_ssm_tab_vi_kind
    fi
    # The second key in the Tab macro invokes the user's previous Tab action.
    if [[ $kind == shell ]]; then
        bind -m "$keymap" -x '"\e[99;2~": '"$action"
        bind -m "$keymap" -x '"\C-i": '"$action"
    else
        bind -m "$keymap" '"\e[99;2~": '"$action"
        bind -m "$keymap" '"\C-i": '"$action"
    fi

    before=${READLINE_LINE:0:READLINE_POINT}
    # Let Readline handle quoted words, expansions, compound commands and pipelines.
    case $before in *[\"\'\`\$\;\&\|\<\>\(\)\{\}\\]*) _ssm_history_last_line=; return ;; esac
    token=${before##*[[:space:]]}
    read -r -a words <<< "${before:0:${#before}-${#token}}"
    command=${words[0]-}
    [[ ${command##*/} == ssm && ${#words[@]} -ge 1 ]] || { _ssm_history_last_line=; return; }
    for word in "${words[@]:1}"; do
        if [[ -n $pending ]]; then
            pending=
            continue
        fi
        case $word in
            -J|--proxy-jump) pending=host ;;
            -i|--identity|-p|--port|-L|--local-forward|-R|--remote-forward|-d|--delete) pending=other ;;
            --) positional=0 ;; # A following target is still a positional argument.
            -*) ;;
            *) positional=1 ;;
        esac
    done
    if [[ $pending == other || ( -z $pending && ( $positional == 1 || $token == -* ) ) ]]; then
        _ssm_history_last_line=
        return
    fi
    # Native completion stays bound for the rest of a prompt after a fallback,
    # preserving Readline's consecutive-action state (e.g. menu-complete).
    # From here, Tab belongs to SSM even when the history has no matching record.
    bind -m "$keymap" '"\C-i": "\e[99;1~\e[99;2~"'
    bind -m "$keymap" '"\e[99;2~": ""'
    if [[ ${_ssm_history_last_line-} != "$READLINE_LINE" || ${_ssm_history_point-} != "$READLINE_POINT" || ${_ssm_history_command-} != "$HISTCMD" ]]; then
        # Drop only the previous provisional insertion when text before it was edited.
        # The original right-hand text remains part of the line.
        local suffix=${READLINE_LINE:READLINE_POINT}
        if [[ ${_ssm_history_command-} == "$HISTCMD" && -n ${_ssm_history_inserted-} && $before != "${_ssm_history_prefix-}" && $suffix == "$_ssm_history_inserted${_ssm_history_suffix-}" ]]; then
            suffix=$_ssm_history_suffix
            READLINE_LINE=$before$suffix
        fi
        _ssm_history_prefix=$before
        _ssm_history_suffix=$suffix
        _ssm_history_inserted=
        _ssm_history_point=$READLINE_POINT
        _ssm_history_command=$HISTCMD
        _ssm_history_token=$token
        _ssm_history_candidates=()
        while IFS= read -r candidate; do
            [[ -n $candidate && $candidate == "$token"* ]] && _ssm_history_candidates+=("$candidate")
        done < <("$command" __history-complete "$token" 2>/dev/null)
        _ssm_history_index=0
    fi
    if (( ${#_ssm_history_candidates[@]} )); then
        candidate=${_ssm_history_candidates[_ssm_history_index]}
        _ssm_history_inserted=${candidate:${#_ssm_history_token}}
        READLINE_LINE=$_ssm_history_prefix$_ssm_history_inserted$_ssm_history_suffix
        READLINE_POINT=$_ssm_history_point
        _ssm_history_index=$(( (_ssm_history_index + 1) % ${#_ssm_history_candidates[@]} ))
    fi
    _ssm_history_last_line=$READLINE_LINE
}

# Remember the previous binding once; sourcing this script again is harmless.
_ssm_history_bind() {
    local keymap=$1 binding action kind=readline
    binding=$(bind -m "$keymap" -p | command grep '^"\\C-i":' || :)
    if [[ -z $binding ]]; then
        binding=$(bind -m "$keymap" -s | command grep '^"\\C-i":' || :)
    fi
    if [[ -z $binding ]]; then
        binding=$(bind -m "$keymap" -X | command grep '^"\\C-i":' || :)
        kind=shell
    fi
    action=${binding#*: }
    [[ -n $binding ]] || action=complete
    if [[ $keymap == emacs-standard ]]; then
        [[ ${_ssm_tab_emacs_action+x} ]] || { _ssm_tab_emacs_action=$action; _ssm_tab_emacs_kind=$kind; }
    else
        [[ ${_ssm_tab_vi_action+x} ]] || { _ssm_tab_vi_action=$action; _ssm_tab_vi_kind=$kind; }
    fi
    bind -m "$keymap" -x '"\e[99;1~": "_ssm_history_tab '"$keymap"'"'
    bind -m "$keymap" '"\C-i": "\e[99;1~\e[99;2~"'
}
_ssm_history_prompt() {
    local status=$?
    _ssm_history_bind emacs-standard
    _ssm_history_bind vi-insert
    return "$status"
}

_ssm_history_install_prompt() {
    local prompt_command
    if [[ $(declare -p PROMPT_COMMAND 2>/dev/null || :) == 'declare -a'* ]]; then
        for prompt_command in "${PROMPT_COMMAND[@]}"; do
            [[ $prompt_command == _ssm_history_prompt ]] && return 0
        done
        PROMPT_COMMAND+=(_ssm_history_prompt)
    else
        case $'\n'${PROMPT_COMMAND-}$'\n' in *$'\n_ssm_history_prompt\n'*) return 0 ;; esac
        PROMPT_COMMAND=${PROMPT_COMMAND:+${PROMPT_COMMAND}$'\n'}_ssm_history_prompt
    fi
}

if [[ $- == *i* ]]; then
    _ssm_history_bind emacs-standard
    _ssm_history_bind vi-insert
    _ssm_history_install_prompt
fi
