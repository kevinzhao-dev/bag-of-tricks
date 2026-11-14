#!/usr/bin/env bash
set -euo pipefail

LABEL="com.kevin.site-monitor-dashboard"
APP_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
PLIST_PATH="${HOME}/Library/LaunchAgents/${LABEL}.plist"
LOG_DIR="${HOME}/Library/Logs"
UID_VALUE="$(id -u)"

mkdir -p "$(dirname -- "$PLIST_PATH")" "$LOG_DIR"

if launchctl print "gui/${UID_VALUE}/${LABEL}" >/dev/null 2>&1; then
    launchctl bootout "gui/${UID_VALUE}" "$PLIST_PATH" >/dev/null 2>&1 || true
fi

cat > "$PLIST_PATH" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>${LABEL}</string>

    <key>ProgramArguments</key>
    <array>
        <string>${APP_DIR}/run-dashboard.sh</string>
    </array>

    <key>WorkingDirectory</key>
    <string>${APP_DIR}</string>

    <key>EnvironmentVariables</key>
    <dict>
        <key>PATH</key>
        <string>/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin</string>
        <key>PYTHONUNBUFFERED</key>
        <string>1</string>
    </dict>

    <key>RunAtLoad</key>
    <true/>

    <key>KeepAlive</key>
    <dict>
        <key>SuccessfulExit</key>
        <false/>
        <key>Crashed</key>
        <true/>
    </dict>

    <key>ThrottleInterval</key>
    <integer>10</integer>

    <key>StandardOutPath</key>
    <string>${LOG_DIR}/site-monitor-dashboard.log</string>

    <key>StandardErrorPath</key>
    <string>${LOG_DIR}/site-monitor-dashboard.err.log</string>
</dict>
</plist>
PLIST

chmod 644 "$PLIST_PATH"
launchctl bootstrap "gui/${UID_VALUE}" "$PLIST_PATH"
launchctl enable "gui/${UID_VALUE}/${LABEL}"
launchctl kickstart -k "gui/${UID_VALUE}/${LABEL}"

cat <<EOF
Installed ${LABEL}
Plist: ${PLIST_PATH}
URL: http://127.0.0.1:8765

Status:
  launchctl print gui/${UID_VALUE}/${LABEL}

Logs:
  tail -f "${LOG_DIR}/site-monitor-dashboard.log"
  tail -f "${LOG_DIR}/site-monitor-dashboard.err.log"

Stop:
  launchctl bootout gui/${UID_VALUE} "${PLIST_PATH}"
EOF
