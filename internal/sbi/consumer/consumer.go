package consumer

import (
	"github.com/free5gc/nef/pkg/app"
	"net/http"

	"github.com/free5gc/nef/internal/logger"
	"github.com/free5gc/openapi"
	"github.com/free5gc/openapi/models"
	Nnrf_NFDiscovery "github.com/free5gc/openapi/nrf/NFDiscovery"
	Nnrf_NFManagement "github.com/free5gc/openapi/nrf/NFManagement"
	Npcf_PolicyAuthorization "github.com/free5gc/openapi/pcf/PolicyAuthorization"
	Nudr_DataRepository "github.com/free5gc/openapi/udr/DataRepository"
)

var consumer *Consumer

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

func handleAPIServiceResponseError(rsp *http.Response, err error) (int, interface{}) {
	var rspCode int
	var rspBody interface{}
	if rsp.Status != err.Error() {
		rspCode, rspBody = handleDeserializeError(rsp, err)
	} else {
		pd := err.(openapi.GenericOpenAPIError).Model().(models.ProblemDetails)
		rspCode, rspBody = int(pd.Status), &pd
	}
	return rspCode, rspBody
}

func handleDeserializeError(rsp *http.Response, err error) (int, interface{}) {
	logger.ConsumerLog.Errorf("Deserialize ProblemDetails Error: %s", err.Error())
	pd := &models.ProblemDetails{
		Status: int32(rsp.StatusCode),
		Detail: err.Error(),
	}
	return int(pd.Status), pd
}

func handleAPIServiceNoResponse(err error) (int, interface{}) {
	detail := "server no response"
	if err != nil {
		detail = err.Error()
	}
	logger.ConsumerLog.Errorf("APIService error: %s", detail)
	pd := openapi.ProblemDetailsSystemFailure(detail)
	return int(pd.Status), pd
}
