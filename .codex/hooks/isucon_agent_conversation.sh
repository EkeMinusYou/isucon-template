#!/bin/sh

export LC_ALL=C

action=${1-}
event=$(cat) || exit 0

git_retry_count=3
git_retry_delay=0.1

git_with_retry() {
  git_attempt=0
  while :; do
    if git "$@"; then
      return 0
    fi
    if [ "$git_attempt" -ge "$git_retry_count" ]; then
      return 1
    fi
    git_attempt=$((git_attempt + 1))
    sleep "$git_retry_delay"
  done
}

if ! command -v jq >/dev/null 2>&1; then
  exit 0
fi

event_value() {
  printf '%s' "$event" | jq -r "$1" 2>/dev/null
}

session_id=$(event_value '.session_id // empty') || exit 0
cwd=$(event_value '.cwd // empty') || exit 0

if [ -z "$session_id" ] || [ -z "$cwd" ]; then
  exit 0
fi

repository_root=$(git_with_retry -C "$cwd" rev-parse --show-toplevel 2>/dev/null) || exit 0
if [ -z "$repository_root" ]; then
  exit 0
fi

root_key=$(printf '%s' "$repository_root" | cksum | awk '{print $1}') || exit 0
safe_session_id=$(printf '%s' "$session_id" | tr -c 'A-Za-z0-9._-' '_')
if [ -z "$safe_session_id" ]; then
  safe_session_id=unknown
fi

state_dir="${TMPDIR:-/tmp}/codex-isucon-agent-session-markers/$root_key"
marker="$state_dir/$safe_session_id.marker"
offset_path="$state_dir/$safe_session_id.offset"
timestamp_path="$state_dir/$safe_session_id.timestamp"
started_at_path="$state_dir/$safe_session_id.started_at"

case "$action" in
  mark)
    if ! printf '%s' "$event" | jq -e '
      (.prompt // empty)
      | select(type == "string")
      | test("(^|[^A-Za-z0-9_-])\\$isucon-agent([^A-Za-z0-9_-]|$)")
    ' >/dev/null 2>&1; then
      exit 0
    fi

    mkdir -p "$state_dir" || exit 0
    if [ ! -f "$timestamp_path" ]; then
      timestamp=$(date '+%Y%m%d-%H%M%S') || exit 0
      started_at=$(date '+%s') || exit 0
      timestamp_temporary_path="$timestamp_path.tmp"
      if ! printf '%s\n' "$timestamp" > "$timestamp_temporary_path"; then
        rm -f "$timestamp_temporary_path"
        exit 0
      fi
      if ! printf '%s\n' "$started_at" > "$started_at_path"; then
        rm -f "$started_at_path" "$timestamp_temporary_path"
        exit 0
      fi
      if ! mv -f "$timestamp_temporary_path" "$timestamp_path"; then
        rm -f "$timestamp_temporary_path" "$started_at_path"
        exit 0
      fi
    fi
    : > "$marker" || exit 0
    chmod 600 "$marker" 2>/dev/null || true
    ;;
  export)
    if [ ! -f "$marker" ]; then
      exit 0
    fi

    transcript_path=$(event_value '.transcript_path // empty') || exit 0
    if [ -z "$transcript_path" ]; then
      exit 0
    fi

    case "$transcript_path" in
      /*) ;;
      *) transcript_path="$cwd/$transcript_path" ;;
    esac

    if [ ! -f "$transcript_path" ]; then
      exit 0
    fi

    timestamp=$(cat "$timestamp_path" 2>/dev/null) || timestamp=
    case "$timestamp" in
      [0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9]-[0-9][0-9][0-9][0-9][0-9][0-9]) ;;
      *)
        timestamp=$(date '+%Y%m%d-%H%M%S') || exit 0
        timestamp_temporary_path="$timestamp_path.tmp"
        if ! printf '%s\n' "$timestamp" > "$timestamp_temporary_path"; then
          rm -f "$timestamp_temporary_path"
          exit 0
        fi
        if ! mv -f "$timestamp_temporary_path" "$timestamp_path"; then
          rm -f "$timestamp_temporary_path"
          exit 0
        fi
        ;;
    esac

    timestamp_prefix="$timestamp"_
    output_dir="$repository_root/docs/conversation"
    output_path="$output_dir/$timestamp_prefix$safe_session_id.md"
    temporary_path="$output_dir/.$timestamp_prefix$safe_session_id.md.tmp"
    delta_path="$state_dir/$safe_session_id.delta"

    mkdir -p "$output_dir" "$state_dir" || exit 0

    offset=0
    if [ -f "$offset_path" ]; then
      offset=$(cat "$offset_path" 2>/dev/null) || offset=
    fi
    case "$offset" in
      ''|*[!0-9]*) offset=0 ;;
    esac

    # Rebuild files created by older exporter formats.
    if [ -f "$output_path" ] && ! grep -q '^<!-- codex-hook-format: 6; tool results omitted -->$' "$output_path"; then
      offset=0
    fi

    # Rebuild files created by older versions that exported internal context.
    if [ -f "$output_path" ] && grep -Eq '^## (DEVELOPER|SYSTEM)$|^# AGENTS\.md instructions for |^<environment_context>$|^<skill>$|^<skills_instructions>$' "$output_path"; then
      offset=0
    fi

    line_count=$(awk 'END { print NR }' "$transcript_path" 2>/dev/null) || exit 0
    case "$line_count" in
      ''|*[!0-9]*) exit 0 ;;
    esac

    if [ "$offset" -gt "$line_count" ] || [ ! -f "$output_path" ]; then
      offset=0
    fi

    if [ "$offset" -eq 0 ]; then
      {
        printf '%s\n\n' "# Codex conversation"
        printf '%s\n\n' "- Session: $safe_session_id"
        printf '%s\n\n' '<!-- codex-hook-format: 6; tool results omitted -->'
        printf '%s\n\n' "---"
      } > "$temporary_path" || exit 0
    else
      if ! cp "$output_path" "$temporary_path"; then
        rm -f "$temporary_path"
        exit 0
      fi
    fi

    started_at=$(cat "$started_at_path" 2>/dev/null) || started_at=
    case "$started_at" in
      ''|*[!0-9]*) ;;
      *)
        stopped_at=$(date '+%s') || exit 0
        if [ "$stopped_at" -ge "$started_at" ]; then
          elapsed_seconds=$((stopped_at - started_at))
          elapsed=$(printf '%02d:%02d:%02d' \
            "$((elapsed_seconds / 3600))" \
            "$(((elapsed_seconds % 3600) / 60))" \
            "$((elapsed_seconds % 60))")
          elapsed_temporary_path="$temporary_path.elapsed"
          if ! awk -v elapsed="$elapsed" '
            /^- Session: / {
              print
              print "- Elapsed: " elapsed
              next
            }
            /^- Elapsed: / { next }
            { print }
          ' "$temporary_path" > "$elapsed_temporary_path"; then
            rm -f "$temporary_path" "$elapsed_temporary_path"
            exit 0
          fi
          mv -f "$elapsed_temporary_path" "$temporary_path" || exit 0
        fi
        ;;
    esac

    rm -f "$delta_path"
    start_line=$((offset + 1))
    # Read earlier records to carry the latest user timestamp into new messages.
    if [ "$start_line" -le "$line_count" ] && ! jq -nr --argjson offset "$offset" '
      def event_time:
        (.timestamp? // .payload.timestamp? // null) as $value
        | if ($value | type) == "number" then $value
          elif ($value | type) == "string" then
            try ($value | sub("\\.[0-9]+Z$"; "Z") | fromdateiso8601) catch null
          else null
          end;

      def response_elapsed($user_at; $time):
        if $user_at == null or $time == null or $time < $user_at then
          null
        else
          ($time - $user_at | floor) as $seconds
          | def pad2: tostring | if length < 2 then "0" + . else . end;
          "\(($seconds / 3600 | floor) | pad2):\((($seconds % 3600) / 60 | floor) | pad2):\(($seconds % 60) | pad2)"
        end;

      def text_from_content:
        if type == "string" then .
        elif type == "array" then
          map(
            if type == "string" then .
            elif type == "object" then
              (.text? // .output_text? // .input_text? // .content? // "") as $value
              | if ($value | type) == "string" then $value
                elif ($value | type) == "array" then ($value | text_from_content)
                else ""
                end
            else ""
            end
          )
          | map(select(length > 0))
          | join("\n")
        elif type == "object" then
          (.text? // .output_text? // .input_text? // .content? // "") as $value
          | if ($value | type) == "string" then $value
            elif ($value | type) == "array" then ($value | text_from_content)
            else ""
            end
        else ""
        end;

      def body:
        (. // "") | text_from_content;

      def trim_outer_newlines:
        sub("^\\n+"; "") | sub("\\n+$"; "");

      def is_internal_user_context:
        startswith("# AGENTS.md instructions for ")
        or startswith("<environment_context>")
        or startswith("<skill>")
        or startswith("<skills_instructions>");

      def user_body:
        if .type? == "user_message" then .message? // .text? // ""
        elif .role? == "user" then .content? // .text? // ""
        else null
        end;

      def section($heading; $value; $elapsed):
        ($value | body | trim_outer_newlines) as $text
        | if ($heading == "USER" and ($text | is_internal_user_context)) then
            empty
          elif ($text | length) > 0 then
            "## \($heading)\(if $heading == "ASSISTANT" and $elapsed != null then " (Elapsed: \($elapsed))" else "" end)\n\n\($text)\n"
          else
            empty
          end;

      def normalized_tool_arguments:
        if . == null then
          null
        elif type == "string" then
          . as $raw_arguments
          | try ($raw_arguments | fromjson) catch $raw_arguments
        else
          .
        end;

      def command_section($command):
        "## TOOL\n\n```sh\n\($command | trim_outer_newlines)\n```\n";

      def command_from_arguments:
        if type != "object" then
          null
        elif (.cmd? | type) == "string" then
          .cmd
        else
          null
        end;

      def tool_section:
        . as $tool
        | ($tool.name? // $tool.tool_name? // "unknown") as $name
        | ($tool.arguments? // $tool.input? // $tool.tool_input? // null) as $raw_arguments
        | ($raw_arguments | normalized_tool_arguments | command_from_arguments) as $command
        | if $name == "exec_command" and $command != null then
            command_section($command)
          elif $raw_arguments == null then
            "## TOOL\n\n### `\($name)`\n"
          else
            ($raw_arguments | normalized_tool_arguments | tojson) as $arguments
            | "## TOOL\n\n### `\($name)`\n\n```json\n\($arguments)\n```\n"
          end;

      def command_from_source($source):
        try (
          $source
          | capture("(?s)(?:\\{|,)\\s*cmd\\s*:\\s*(?<value>\"(?:\\\\.|[^\"\\\\])*\")")
          | .value
          | fromjson
        ) catch null;

      def matching_parenthesis($source; $index; $depth; $quote; $escaped):
        if $index >= ($source | length) then
          null
        else
          $source[$index:($index + 1)] as $character
          | if $quote != null then
              if $escaped then
                matching_parenthesis($source; ($index + 1); $depth; $quote; false)
              elif $character == "\\" then
                matching_parenthesis($source; ($index + 1); $depth; $quote; true)
              elif $character == $quote then
                matching_parenthesis($source; ($index + 1); $depth; null; false)
              else
                matching_parenthesis($source; ($index + 1); $depth; $quote; false)
              end
            elif ($character == "\"" or $character == "\u0027" or $character == "`") then
              matching_parenthesis($source; ($index + 1); $depth; $character; false)
            elif $character == "(" then
              matching_parenthesis($source; ($index + 1); ($depth + 1); null; false)
            elif $character == ")" then
              if $depth == 1 then
                $index
              else
                matching_parenthesis($source; ($index + 1); ($depth - 1); null; false)
              end
            else
              matching_parenthesis($source; ($index + 1); $depth; null; false)
            end
        end;

      def nested_tool_calls($source; $from):
        ($source[$from:] | index("tools.")) as $relative
        | if $relative == null then
            []
          else
            ($from + $relative) as $start
            | ($source[($start + 6):]
              | try capture("^(?<name>[A-Za-z_][A-Za-z0-9_]*)\\(") catch null) as $match
            | if $match == null then
                nested_tool_calls($source; ($start + 6))
              else
                ($start + 6 + ($match.name | length)) as $open
                | matching_parenthesis($source; ($open + 1); 1; null; false) as $close
                | if $close == null then
                    []
                  else
                    [{name: $match.name, source: $source[$start:($close + 1)]}]
                    + nested_tool_calls($source; ($close + 1))
                  end
              end
          end;

      def nested_tool_section:
        . as $call
        | if $call.name == "exec_command" then
            ($call.source | command_from_source(.)) as $command
            | if $command != null then
                command_section($command)
              else
                "## TOOL\n\n### `exec_command`\n\n```javascript\n\($call.source | trim_outer_newlines)\n```\n"
              end
          else
            "## TOOL\n\n### `\($call.name)`\n\n```javascript\n\($call.source | trim_outer_newlines)\n```\n"
          end;

      def code_mode_tool_section:
        . as $tool
        | ($tool.arguments? // $tool.input? // $tool.tool_input? // null) as $raw_arguments
        | if ($tool.name? == "exec" or $tool.name? == "functions.exec")
             and (($raw_arguments | type) == "string") then
            if ($raw_arguments | contains("tools.")) then
              ($raw_arguments | nested_tool_calls(.; 0)) as $calls
              | if ($calls | length) > 0 then
                  $calls
                  | map(nested_tool_section)
                  | join("\n")
                else
                  ($tool | tool_section)
                end
            else
              ($tool | tool_section)
            end
          else
            ($tool | tool_section)
          end;

      def render($user_at; $time):
        . as $payload
        | response_elapsed($user_at; $time) as $elapsed
        | if ($payload.type? == "message"
            and ($payload.role? == "user"
              or $payload.role? == "assistant")) then
          section(($payload.role | ascii_upcase); ($payload.content? // $payload.text? // ""); $elapsed)
        elif ($payload.type? == "user_message") then
          section("USER"; ($payload.message? // $payload.text? // ""); null)
        elif ($payload.type? == "agent_message") then
          section("ASSISTANT"; ($payload.message? // $payload.text? // ""); $elapsed)
        elif ($payload.type? == "function_call"
              or $payload.type? == "custom_tool_call"
              or $payload.type? == "tool_call"
              or $payload.type? == "tool_use") then
          ($payload | code_mode_tool_section)
        elif ($payload.role? == "user" or $payload.role? == "assistant") then
          section(($payload.role | ascii_upcase); ($payload.content? // $payload.text? // ""); $elapsed)
        else
          empty
        end;

      foreach inputs as $record (
        {line: 0, user_at: null, output: ""};
        .line += 1
        | ($record.payload // $record) as $payload
        | ($record | event_time) as $time
        | ($payload | user_body) as $user_text
        | if $user_text != null and (($user_text | body | is_internal_user_context) | not) then
            .user_at = $time
          else . end
        | .user_at as $user_at
        | .output = if .line > $offset then
            ([$payload | render($user_at; $time)] | join(""))
          else "" end;
        .output | select(length > 0)
      )
    ' "$transcript_path" > "$delta_path"; then
      rm -f "$temporary_path"
      rm -f "$delta_path"
      exit 0
    fi

    if [ -s "$delta_path" ] && ! cat "$delta_path" >> "$temporary_path"; then
      rm -f "$temporary_path" "$delta_path"
      exit 0
    fi

    output_changed=0
    if [ ! -f "$output_path" ] || ! cmp -s "$temporary_path" "$output_path"; then
      output_changed=1
    fi

    if ! mv -f "$temporary_path" "$output_path"; then
      rm -f "$temporary_path"
      rm -f "$delta_path"
      exit 0
    fi

    offset_temporary_path="$offset_path.tmp"
    if ! printf '%s\n' "$line_count" > "$offset_temporary_path"; then
      rm -f "$delta_path" "$offset_temporary_path"
      exit 0
    fi
    if ! mv -f "$offset_temporary_path" "$offset_path"; then
      rm -f "$delta_path" "$offset_temporary_path"
      exit 0
    fi

    if [ "$output_changed" -eq 1 ]; then
      output_relative_path=${output_path#"$repository_root"/}
      if git_with_retry -C "$repository_root" add -- "$output_relative_path" >/dev/null 2>&1; then
        git_with_retry -C "$repository_root" commit --only \
          -m "docs(conversation): save session $safe_session_id" \
          -- "$output_relative_path" >/dev/null 2>&1 || true
      fi
    fi

    rm -f "$delta_path"
    ;;
  cleanup)
    rm -f "$marker" "$offset_path" "$started_at_path" "$state_dir/$safe_session_id.delta"
    timestamp=$(cat "$timestamp_path" 2>/dev/null) || timestamp=
    timestamp_prefix="$timestamp"_
    rm -f "$repository_root/docs/conversation/.$timestamp_prefix$safe_session_id.md.tmp"
    rm -f "$timestamp_path"
    ;;
esac

exit 0
