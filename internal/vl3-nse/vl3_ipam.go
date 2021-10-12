package vl3_nse

import (
	"context"
	"fmt"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/networkservicemesh/api/pkg/api/networkservice"
	"github.com/networkservicemesh/sdk/pkg/networkservice/core/next"
	"net"
)

type vl3IpExcludeServer struct {
	//Map
	ipamPrefix  *net.IPNet
	reserveAddrs []net.IP
}

// NewServer - creates a new NetworkServiceServer chain element that implements IPAM service.
func NewVl3IpExcludeServer(ipamPrefix *net.IPNet, reservedIps ...net.IP) networkservice.NetworkServiceServer {
	return &vl3IpExcludeServer{
		ipamPrefix: ipamPrefix,
		reserveAddrs: reservedIps,
	}
}

func (s *vl3IpExcludeServer) Request(ctx context.Context, request *networkservice.NetworkServiceRequest) (*networkservice.Connection, error) {

	conn := request.GetConnection()
	if conn.GetContext() == nil {
		conn.Context = &networkservice.ConnectionContext{}
	}
	if conn.GetContext().GetIpContext() == nil {
		//conn.GetContext().IpContext = &networkservice.IPContext{}
		return nil, fmt.Errorf("vl3Server requires IPAM chain element in network service endpoint")
	}
	ipContext := conn.GetContext().GetIpContext()

	for _, ipAddr := range s.reserveAddrs {
		ipContext.ExcludedPrefixes = append(ipContext.ExcludedPrefixes, fmt.Sprintf("%s/32", ipAddr.String()))
	}
	conn, err := next.Server(ctx).Request(ctx, request)
	if err != nil {
		return nil, err
	}

	return conn, nil
}

func (s *vl3IpExcludeServer) Close(ctx context.Context, conn *networkservice.Connection) (_ *empty.Empty, err error) {
	return next.Server(ctx).Close(ctx, conn)
}


