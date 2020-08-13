module github.com/smartwalle/net4go/cmd

require (
	github.com/gorilla/websocket v1.4.2
	github.com/smartwalle/net4go v0.0.33
	github.com/smartwalle/net4go/ws v0.0.3
	github.com/smartwalle/net4go/quic v0.0.0
)

replace github.com/smartwalle/net4go/quic => ../quic

go 1.12
