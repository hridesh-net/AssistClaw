#!/usr/bin/expect
spawn ssh deros@100.96.25.105 "journalctl --user -u assistclaw --since '15 minutes ago' --no-pager | grep -E 'whatsapp|runner|tool'"
expect "password:"
send "123456\r"
expect eof
