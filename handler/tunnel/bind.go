package tunnel

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net"

	"github.com/go-gost/core/logger"
	"github.com/go-gost/relay"
	"github.com/go-gost/x/internal/util/mux"
	"github.com/google/uuid"
)

func (h *tunnelHandler) handleBind(ctx context.Context, conn net.Conn, network, address string, tunnelID relay.TunnelID, log logger.Logger) (err error) {
	resp := relay.Response{
		Version: relay.Version1,
		Status:  relay.StatusOK,
	}

	uuid, err := uuid.NewRandom()
	if err != nil {
		resp.Status = relay.StatusInternalServerError
		resp.WriteTo(conn)
		return
	}
	connectorID := relay.NewConnectorID(uuid[:])
	if network == "udp" {
		connectorID = relay.NewUDPConnectorID(uuid[:])
	}

	addr := address
	if host, port, _ := net.SplitHostPort(addr); host == "" {
		v := md5.Sum([]byte(tunnelID.String()))
		host = hex.EncodeToString(v[:8])
		addr = net.JoinHostPort(host, port)
	}
	af := &relay.AddrFeature{}
	err = af.ParseFrom(addr)
	if err != nil {
		log.Warn(err)
	}
	resp.Features = append(resp.Features, af,
		&relay.TunnelFeature{
			ID: connectorID.ID(),
		},
	)
	resp.WriteTo(conn)

	// Upgrade connection to multiplex session.
	session, err := mux.ClientSession(conn, h.md.muxCfg)
	if err != nil {
		return
	}

	h.pool.Add(tunnelID, NewConnector(connectorID, session))
	if h.md.ingress != nil {
		h.md.ingress.Set(ctx, addr, tunnelID.String())
	}
	if sd := h.md.sd; sd != nil {
		err := sd.Register(ctx, fmt.Sprintf("%s:%s:%s", h.id, tunnelID, connectorID), network, h.md.entryPoint)
		if err != nil {
			h.log.Error(err)
		}
	}

	log.Debugf("%s/%s: tunnel=%s, connector=%s established", addr, network, tunnelID, connectorID)

	return
}
