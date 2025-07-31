package consumer

import (
	"github.com/free5gc/nef/pkg/app"
	Nnrf_NFDiscovery "github.com/free5gc/openapi/nrf/NFDiscovery"
	Nnrf_NFManagement "github.com/free5gc/openapi/nrf/NFManagement"
	Npcf_PolicyAuthorization "github.com/free5gc/openapi/pcf/PolicyAuthorization"
	Nudr_DataRepository "github.com/free5gc/openapi/udr/DataRepository"
)

type ConsumerNef interface {
	app.App
}

type Consumer struct {
	ConsumerNef

	// consumer services
	*nnrfService
	*npcfService
	*nudrService
}

func NewConsumer(nef ConsumerNef) (*Consumer, error) {
	c := &Consumer{
		ConsumerNef: nef,
	}

	c.nnrfService = &nnrfService{
		consumer:        c,
		nfDiscClients:   make(map[string]*Nnrf_NFDiscovery.APIClient),
		nfMngmntClients: make(map[string]*Nnrf_NFManagement.APIClient),
	}

	c.npcfService = &npcfService{
		consumer: c,
		clients:  make(map[string]*Npcf_PolicyAuthorization.APIClient),
	}

	c.nudrService = &nudrService{
		consumer: c,
		clients:  make(map[string]*Nudr_DataRepository.APIClient),
	}

	return c, nil
}
