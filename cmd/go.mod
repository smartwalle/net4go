module github.com/smartwalle/net4go/cmd

require (
	github.com/golang/protobuf v1.4.2
	github.com/gorilla/websocket v1.4.2
	github.com/smartwalle/net4go v0.0.39
	github.com/smartwalle/net4go/quic v0.0.4
	github.com/smartwalle/net4go/ws v0.0.10
)

replace (
	github.com/smartwalle/net4go => ../
	github.com/smartwalle/net4go/quic => ../quic
	github.com/smartwalle/net4go/ws => ../ws
)

go 1.12

