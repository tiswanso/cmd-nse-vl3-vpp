package vl3_nse

import (
	"context"
	"fmt"
	"github.com/edwarnicke/grpcfd"
	"github.com/edwarnicke/vpphelper"
	"github.com/networkservicemesh/api/pkg/api/networkservice"
	"github.com/networkservicemesh/api/pkg/api/registry"
	"github.com/networkservicemesh/sdk-vpp/pkg/networkservice/connectioncontext"
	"github.com/networkservicemesh/sdk-vpp/pkg/networkservice/mechanisms/memif"
	"github.com/networkservicemesh/sdk-vpp/pkg/networkservice/up"
	"github.com/networkservicemesh/sdk/pkg/networkservice/chains/client"
	"github.com/networkservicemesh/sdk/pkg/networkservice/common/mechanisms/recvfd"
	"github.com/networkservicemesh/sdk/pkg/networkservice/common/mechanisms/sendfd"
	"github.com/networkservicemesh/sdk/pkg/networkservice/utils/metadata"
	"github.com/networkservicemesh/sdk/pkg/tools/nsurl"
	//"github.com/onsi/gomega/format"

	//"github.com/networkservicemesh/sdk/pkg/tools/nsurl"
	"github.com/networkservicemesh/sdk/pkg/tools/opentracing"
	"github.com/networkservicemesh/sdk/pkg/tools/spiffejwt"
	"github.com/networkservicemesh/sdk/pkg/tools/token"
	"github.com/spiffe/go-spiffe/v2/spiffetls/tlsconfig"
	"github.com/spiffe/go-spiffe/v2/workloadapi"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"net/url"
	"time"

	//"github.com/networkservicemesh/sdk/pkg/tools/clock"
	"github.com/networkservicemesh/sdk/pkg/tools/log"
)

type NsePeering struct {
	ctx         context.Context
	selfNseName string
	networkSvc  string
	x509Src     *workloadapi.X509Source
	vppConn     vpphelper.Connection
	nsClient    networkservice.NetworkServiceClient
	nsmConnectTo url.URL
	connectedNses map[string]registry.NetworkServiceEndpoint
}

func NewNsePeering(netsvcName, myNseName string,
	x509Src *workloadapi.X509Source, vppConn vpphelper.Connection,
	nsmConnectTo url.URL, dialTimeout time.Duration, maxTokenLifetime time.Duration ) *NsePeering {
	ctx := context.Background()
	dialOptions := append(opentracing.WithTracingDial(),
		grpc.WithDefaultCallOptions(
			grpc.WaitForReady(true),
			grpc.PerRPCCredentials(token.NewPerRPCCredentials(spiffejwt.TokenGeneratorFunc(x509Src, maxTokenLifetime))),
		),
		grpc.WithTransportCredentials(
			grpcfd.TransportCredentials(
				credentials.NewTLS(
					tlsconfig.MTLSClientConfig(x509Src, x509Src, tlsconfig.AuthorizeAny()),
				),
			),
		),
		grpcfd.WithChainStreamInterceptor(),
		grpcfd.WithChainUnaryInterceptor(),
	)

	c := client.NewClient(
		ctx,
		&nsmConnectTo,
		client.WithName(myNseName),
		client.WithAdditionalFunctionality(
			metadata.NewClient(),
			up.NewClient(ctx, vppConn),
			connectioncontext.NewClient(vppConn),
			memif.NewClient(vppConn),
			sendfd.NewClient(),
			recvfd.NewClient(),
		),
		client.WithDialTimeout(dialTimeout),
		client.WithDialOptions(dialOptions...),
	)

	return &NsePeering {
		ctx: ctx,
		selfNseName: myNseName,
		networkSvc: netsvcName,
		x509Src: x509Src,
		vppConn: vppConn,
		nsClient: c,
		nsmConnectTo: nsmConnectTo,
	}
}

func (n *NsePeering) DoNSEPeering(nseRegistryClient registry.NetworkServiceEndpointRegistryClient) {
	//ctx := context.Background()
	//clockTime := clock.FromContext(ctx)

	query := &registry.NetworkServiceEndpointQuery{
		NetworkServiceEndpoint: &registry.NetworkServiceEndpoint{
			NetworkServiceNames: []string{n.networkSvc},
		},
	}
	/*
	nseStream, err := nseRegistryClient.Find(n.ctx, query)
	if err != nil {
		//return nil, errors.WithStack(err)
		log.Default().Errorf("Failed to find NSE endpoints for network service \"%s\": %v",
			n.networkSvc, err)
		return
	}
	nseList := registry.ReadNetworkServiceEndpointList(nseStream)

	for _, nse := range nseList {
		if nse.Name != n.selfNseName {
			log.Default().Infof("Found peer NSE %s", nse.Name)
			n.ProcessPeer(nse)
		} else {
			log.Default().Infof("Found my NSE %s", nse.Name)
		}
	}

	 */

	query.Watch = true

	//ctx, cancelFind := context.WithCancel(ctx)
	//defer cancelFind()

	nseStream, err := nseRegistryClient.Find(n.ctx, query)
	if err != nil {
		//return nil, errors.WithStack(err)
		log.Default().Errorf("Failed to watch NSE endpoints for network service \"%s\": %v",
			n.networkSvc, err)
	}

	for {
		var nse *registry.NetworkServiceEndpoint
		if nse, err = nseStream.Recv(); err != nil {
			log.Default().Errorf("Failed to receive NSE endpoints stream for network service \"%s\": %v",
				n.networkSvc, err)
			return
		}
		log.Default().Infof("Received stream info for NSE %s", nse.Name)
		if nse.Name != n.selfNseName {
			log.Default().Infof("Found peer NSE %s", nse.Name)
			n.ProcessPeer(nse)
			break
		} else {
			log.Default().Infof("Found my NSE %s", nse.Name)
		}
		//result = matchEndpoint(clockTime, labels, ns, nse)
		//if len(result) != 0 {
		//	return result, nil
		//}
	}
}

func (n *NsePeering) ProcessPeer(nse *registry.NetworkServiceEndpoint) {
	if n.selfNseName > nse.Name {
		log.Default().Infof("Self NSE name larger than peer NSE... initiate connection to %s", nse.Name)
		err := n.doNsConnect(nse)
		if err != nil {
			log.Default().Errorf("Failed to connect to NSE %s: %v", nse.Name, err)
		}
	}
}

func (n *NsePeering) doNsConnect(nse *registry.NetworkServiceEndpoint) error {
	ctx := context.Background()
	netSvcUrl, err := url.Parse("memif://" + n.networkSvc + "/nsm-2")
	if err != nil {
		log.FromContext(ctx).Errorf("NetworkService name to URL parse failed for %s: %v", n.networkSvc, err.Error())
		return err
	}
	u := nsurl.NSURL(*netSvcUrl)
	mech := u.Mechanism()
	if mech.Type != memif.MECHANISM {
		log.FromContext(ctx).Errorf("mechanism type: %v is not supproted", mech.Type)
		return fmt.Errorf("mechanism type: %v is not supproted", mech.Type)
	}
	request := &networkservice.NetworkServiceRequest{
		Connection: &networkservice.Connection{
			NetworkService: u.NetworkService(),
			Labels:         u.Labels(),
			NetworkServiceEndpointName: nse.Name,
		},
	}

	requestCtx, cancelRequest := context.WithTimeout(ctx, 2*time.Minute)
	defer cancelRequest()

	resp, err := n.nsClient.Request(requestCtx, request)
	if err != nil {
		log.FromContext(ctx).Errorf("request has failed: %v", err.Error())
		return err
	}

	defer func() {
		closeCtx, cancelClose := context.WithTimeout(ctx, 2*time.Minute)
		defer cancelClose()
		_, _ = n.nsClient.Close(closeCtx, resp)
	}()

	return nil
}

