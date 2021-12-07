module github.com/smartwalle/net4go/cmd

require (
	github.com/gorilla/websocket v1.4.2
	github.com/smartwalle/net4go v0.0.44
	github.com/smartwalle/net4go/quic v0.0.4
	github.com/smartwalle/net4go/ws v0.0.10
	go.uber.org/ratelimit v0.2.0 // indirect
)

replace (
	github.com/smartwalle/net4go => ../
	github.com/smartwalle/net4go/quic => ../quic
	github.com/smartwalle/net4go/ws => ../ws
)

go 1.12
