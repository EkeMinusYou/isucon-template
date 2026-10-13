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
timestamp_path="$state_dir/$safe_session_id.timestamp"

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
      timestamp_temporary_path="$timestamp_path.tmp"
      if ! printf '%s\n' "$timestamp" > "$timestamp_temporary_path"; then
        rm -f "$timestamp_temporary_path"
        exit 0
      fi
      if ! mv -f "$timestamp_temporary_path" "$timestamp_path"; then
        rm -f "$timestamp_temporary_path"
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

    # Serialize background exports that share a session output file.
    mkdir -p "$state_dir" || exit 0
    export_lock="$state_dir/$safe_session_id.export.lock"
    export_attempt=0
    while ! mkdir "$export_lock" 2>/dev/null; do
      if [ "$export_attempt" -ge 200 ]; then
        exit 0
      fi
      export_attempt=$((export_attempt + 1))
      sleep 0.1
    done
    trap 'rmdir "$export_lock" 2>/dev/null || true' 0
    trap 'exit 0' HUP INT TERM

    # Stop runs before task_complete is written; the hook runs asynchronously.
    turn_id=$(event_value '.turn_id // empty') || turn_id=
    if [ -n "$turn_id" ]; then
      completion_attempt=0
      while ! tail -n 50 "$transcript_path" | jq -e --arg turn_id "$turn_id" '
        select(.payload.type? == "task_complete" and .payload.turn_id? == $turn_id)
      ' >/dev/null 2>&1; do
        if [ "$completion_attempt" -ge 50 ]; then
          break
        fi
        completion_attempt=$((completion_attempt + 1))
        sleep 0.1
      done
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
    mkdir -p "$output_dir" || exit 0

    {
      printf '%s\n\n' "# Codex conversation"
      printf '%s\n\n' "- Session: $safe_session_id"
      printf '%s\n\n' '<!-- codex-hook-format: 11; tool results and reasoning content omitted -->'
      printf '%s\n\n' '- REASONING durations are estimates from the preceding message or tool event to reasoning completion and may include model/network waits.'
      printf '%s\n\n' '- SETTINGS record observed configuration changes, not the service tier actually used by each request. Unknown values were not recorded; later observations do not backfill earlier settings.'
      printf '%s\n\n' "---"
    } > "$temporary_path" || exit 0

    # Rebuild to include completion records appended after an earlier export.
    if ! jq -sr '
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
          ($time - $user_at | floor)
        end;

      def worked_for:
        . as $seconds
        | [
            if $seconds >= 3600 then "\($seconds / 3600 | floor)h" else empty end,
            if $seconds >= 60 then "\(($seconds % 3600) / 60 | floor)m" else empty end,
            "\($seconds % 60)s"
          ] | join(" ");

      def is_final_response:
        (.phase? // .channel? // null) as $phase
        | $phase == "final_answer" or $phase == "final" or $phase == null;

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

      def section($heading; $value; $elapsed; $time):
        ($value | body | trim_outer_newlines) as $text
        | if ($heading == "USER" and ($text | is_internal_user_context)) then
            empty
          elif ($text | length) > 0 then
            [
              if $heading == "ASSISTANT" and $elapsed != null then "Worked for \($elapsed | worked_for)" else empty end,
              if $time != null and ($heading == "USER" or ($heading == "ASSISTANT" and $elapsed != null)) then
                ($time | strflocaltime("%Y-%m-%d %H:%M:%S %z"))
              else empty end
            ] as $timing
            | "## \($heading)\(if ($timing | length) > 0 then " (\($timing | join("; ")))" else "" end)\n\n\($text)\n"
          else
            empty
          end;

      def is_timed_activity:
        .type? as $type
        | if (.role? == "user" or $type == "user_message") then
            (user_body | body | is_internal_user_context | not)
          else
            .role? == "assistant"
            or (["agent_message", "reasoning", "function_call", "custom_tool_call",
                 "tool_call", "tool_use", "function_call_output", "custom_tool_call_output",
                 "tool_result"] | index($type)) != null
          end;

      def reasoning_section($elapsed; $time):
        [
          if $elapsed != null then "Estimated \($elapsed | worked_for)" else "Duration unknown" end,
          if $time != null then ($time | strflocaltime("%Y-%m-%d %H:%M:%S %z")) else empty end
        ] | "## REASONING (\(join("; ")))\n";

      def settings_patch:
        (.payload // .) as $payload
        | (if .type? == "turn_context" then $payload
           elif $payload.type? == "thread_settings_applied" then
             ($payload.thread_settings? // {})
           else {} end)
        | if type == "object" then
            with_entries(select(.key == "model" or .key == "effort"
              or .key == "reasoning_effort" or .key == "service_tier"))
            | if has("reasoning_effort") then
                .effort = .reasoning_effort | del(.reasoning_effort)
              else . end
            | map_values(if type == "string" and length > 0 then . else null end)
          else {} end;

      def settings_section($time):
        "## SETTINGS\(if $time != null then " (\($time | strflocaltime("%Y-%m-%d %H:%M:%S %z")))" else "" end)\n\n"
        + "- Model: \(.model // "unknown")\n"
        + "- Reasoning effort: \(.effort // "unknown")\n"
        + "- Service tier: \(.service_tier // "unknown")\n";

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

      def render($elapsed; $time; $activity_elapsed):
        . as $payload
        | if $payload.type? == "reasoning" then
          reasoning_section($activity_elapsed; $time)
        elif ($payload.type? == "message"
            and ($payload.role? == "user"
              or $payload.role? == "assistant")) then
          section(($payload.role | ascii_upcase); ($payload.content? // $payload.text? // ""); $elapsed; $time)
        elif ($payload.type? == "user_message") then
          section("USER"; ($payload.message? // $payload.text? // ""); null; $time)
        elif ($payload.type? == "agent_message") then
          section("ASSISTANT"; ($payload.message? // $payload.text? // ""); $elapsed; $time)
        elif ($payload.type? == "function_call"
              or $payload.type? == "custom_tool_call"
              or $payload.type? == "tool_call"
              or $payload.type? == "tool_use") then
          ($payload | code_mode_tool_section)
        elif ($payload.role? == "user" or $payload.role? == "assistant") then
          section(($payload.role | ascii_upcase); ($payload.content? // $payload.text? // ""); $elapsed; $time)
        else
          empty
        end;

      . as $records
      | (reduce $records[] as $record (
          {};
          ($record.payload // $record) as $payload
          | if $payload.type? == "task_complete"
               and ($payload.turn_id? | type) == "string"
               and ($payload.duration_ms? | type) == "number"
               and $payload.duration_ms >= 0 then
              .[$payload.turn_id] = ($payload.duration_ms / 1000 | floor)
            else . end
        )) as $completed_turns
      | foreach $records[] as $record (
        {user_at: null, activity_at: null, turn_id: null, output: "",
         settings: {model: null, effort: null, service_tier: null}, settings_seen: false};
        ($record.payload // $record) as $payload
        | ($record | event_time) as $time
        | ($record | settings_patch) as $settings_patch
        | .settings as $previous_settings
        | .settings += $settings_patch
        | (if ($settings_patch | length) > 0
               and ((.settings_seen | not) or .settings != $previous_settings) then
             (.settings | settings_section($time))
           else "" end) as $settings_output
        | if ($settings_patch | length) > 0 then .settings_seen = true else . end
        | if $payload.type? == "task_started" then
            .turn_id = $payload.turn_id
            | .activity_at = $time
          elif $payload.type? == "task_complete" then
            .activity_at = null
          else . end
        | ($payload | user_body) as $user_text
        | if $user_text != null and (($user_text | body | is_internal_user_context) | not) then
            .user_at = $time
          else . end
        | .user_at as $user_at
        | ($payload.internal_chat_message_metadata_passthrough.turn_id?
           // $payload.turn_id? // .turn_id) as $response_turn
        | (if ($payload | is_final_response) then
            (if $response_turn != null then $completed_turns[$response_turn] else null end)
            // response_elapsed($user_at; $time)
          else null end) as $elapsed
        | response_elapsed(.activity_at; $time) as $activity_elapsed
        | .output = $settings_output
            + ([$payload | render($elapsed; $time; $activity_elapsed)] | join(""))
        | if ($payload | is_timed_activity) then
            .activity_at = $time
          else . end;
        .output | select(length > 0)
      )
    ' "$transcript_path" >> "$temporary_path"; then
      rm -f "$temporary_path"
      exit 0
    fi

    output_changed=0
    if [ ! -f "$output_path" ] || ! cmp -s "$temporary_path" "$output_path"; then
      output_changed=1
    fi

    if ! mv -f "$temporary_path" "$output_path"; then
      rm -f "$temporary_path"
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

    ;;
  cleanup)
    rm -f "$marker"
    rmdir "$state_dir/$safe_session_id.export.lock" 2>/dev/null || true
    timestamp=$(cat "$timestamp_path" 2>/dev/null) || timestamp=
    timestamp_prefix="$timestamp"_
    rm -f "$repository_root/docs/conversation/.$timestamp_prefix$safe_session_id.md.tmp"
    rm -f "$timestamp_path"
    ;;
esac

exit 0
