package consumer

import (
	"context"
	"fmt"
	nef_context "github.com/free5gc/nef/internal/context"
	"strings"
	"sync"
	"time"

	"github.com/free5gc/nef/internal/logger"
	"github.com/free5gc/openapi"
	"github.com/free5gc/openapi/models"
	Nnrf_NFDiscovery "github.com/free5gc/openapi/nrf/NFDiscovery"
	Nnrf_NFManagement "github.com/free5gc/openapi/nrf/NFManagement"
)

const (
	RetryRegisterNrfDuration = 2 * time.Second
)

type nnrfService struct {
	consumer *Consumer

	nfDiscMu   sync.RWMutex
	nfMngmntMu sync.RWMutex

	nfDiscClients   map[string]*Nnrf_NFDiscovery.APIClient
	nfMngmntClients map[string]*Nnrf_NFManagement.APIClient
}

func (s *nnrfService) getNFDiscClient(uri string) *Nnrf_NFDiscovery.APIClient {
	if uri == "" {
		return nil
	}

	s.nfDiscMu.RLock()
	if client, ok := s.nfDiscClients[uri]; ok {
		defer s.nfDiscMu.RUnlock()
		return client
	}

	configuration := Nnrf_NFDiscovery.NewConfiguration()
	configuration.SetBasePath(uri)
	cli := Nnrf_NFDiscovery.NewAPIClient(configuration)

	s.nfDiscMu.RUnlock()
	s.nfDiscMu.Lock()
	defer s.nfDiscMu.Unlock()
	s.nfDiscClients[uri] = cli
	return cli
}

func (s *nnrfService) getNFManagementClient(uri string) *Nnrf_NFManagement.APIClient {
	if uri == "" {
		return nil
	}

	s.nfMngmntMu.RLock()
	if client, ok := s.nfMngmntClients[uri]; ok {
		defer s.nfMngmntMu.RUnlock()
		return client
	}

	configuration := Nnrf_NFManagement.NewConfiguration()
	configuration.SetBasePath(uri)
	cli := Nnrf_NFManagement.NewAPIClient(configuration)

	s.nfMngmntMu.RUnlock()
	s.nfMngmntMu.Lock()
	defer s.nfMngmntMu.Unlock()
	s.nfMngmntClients[uri] = cli
	return cli
}

func (s *nnrfService) SendSearchNFInstances(nrfUri string, targetNfType, requestNfType models.NrfNfManagementNfType,
	param *Nnrf_NFDiscovery.SearchNFInstancesRequest,
) (*models.SearchResult, error) {
	// Set client and set url
	param.TargetNfType = &targetNfType
	param.RequesterNfType = &requestNfType
	client := s.getNFDiscClient(nrfUri)
	if client == nil {
		return nil, openapi.ReportError("nrf not found")
	}

	ctx, _, err := s.consumer.Context().GetTokenCtx(models.ServiceName_NNRF_DISC, models.NrfNfManagementNfType_NRF)
	if err != nil {
		return nil, err
	}
	res, err := client.NFInstancesStoreApi.SearchNFInstances(ctx, param)
	var result *models.SearchResult
	if err != nil {
		logger.ConsumerLog.Errorf("SearchNFInstances failed: %+v", err)
	}
	if res != nil {
		result = &res.SearchResult
	}
	return result, err
}

func (s *nnrfService) BuildNFInstance(context *nef_context.NefContext) (
	profile models.NrfNfManagementNfProfile, err error,
) {
	profile.NfInstanceId = context.GetNfInstID()
	profile.NfType = models.NrfNfManagementNfType_NEF
	profile.NfStatus = models.NrfNfManagementNfStatus_REGISTERED

	if context.GetRegisterIPv4() == "" {
		err = fmt.Errorf("NEF Address is empty")
		return profile, err
	}

	profile.Ipv4Addresses = append(profile.Ipv4Addresses, context.GetRegisterIPv4())

	var service []models.NrfNfManagementNfService
	for _, nfService := range context.GetNfServices() {
		service = append(service, nfService)
	}
	if len(service) > 0 {
		profile.NfServices = service
	}

	return profile, err
}

func (s *nnrfService) SendRegisterNFInstance(ctx context.Context, nrfUri, nfInstanceId string,
	profile *models.NrfNfManagementNfProfile) (
	resourceNrfUri string, retrieveNfInstanceId string, err error,
) {
	// Set client and set url
	client := s.getNFManagementClient(nrfUri)
	if client == nil {
		return "", "", openapi.ReportError("nrf not found")
	}

	var res *Nnrf_NFManagement.RegisterNFInstanceResponse
	var nf models.NrfNfManagementNfProfile
	registerNFInstanceRequest := &Nnrf_NFManagement.RegisterNFInstanceRequest{
		NfInstanceID:             &nfInstanceId,
		NrfNfManagementNfProfile: profile,
	}
	finish := false
	for !finish {
		select {
		case <-ctx.Done():
			return "", "", fmt.Errorf("context done")
		default:
			res, err = client.NFInstanceIDDocumentApi.RegisterNFInstance(ctx, registerNFInstanceRequest)
			if err != nil || res == nil {
				logger.ConsumerLog.Errorf("NEF register to NRF Error[%s]", err.Error())
				time.Sleep(RetryRegisterNrfDuration)
				continue
			}
			if res.Location == "" {
				// NFUpdate
				finish = true
			} else {
				// NFRegister
				resourceUri := res.Location
				nf = res.NrfNfManagementNfProfile
				index := strings.Index(resourceUri, "/nnrf-nfm/")
				if index >= 0 {
					resourceNrfUri = resourceUri[:index]
				}
				retrieveNfInstanceId = resourceUri[strings.LastIndex(resourceUri, "/")+1:]

				oauth2 := false
				if nf.CustomInfo != nil {
					v, ok := nf.CustomInfo["oauth2"].(bool)
					if ok {
						oauth2 = v
						logger.MainLog.Infoln("OAuth2 setting receive from NRF:", oauth2)
					}
				}
				nefCtx := s.consumer.ConsumerNef.Context()
				nefCtx.OAuth2Required = oauth2
				if oauth2 && nefCtx.GetNrfCertPem() == "" {
					logger.CfgLog.Error("OAuth2 enable but no nrfCertPem provided in config.")
				}
				finish = true
			}
		}
	}
	return resourceNrfUri, retrieveNfInstanceId, err
}

func (s *nnrfService) SendDeregisterNFInstance() (problemDetails *models.ProblemDetails, err error) {
	logger.ConsumerLog.Infof("[NEF] Send Deregister NFInstance")
	nefContext := s.consumer.Context()

	client := s.getNFManagementClient(nefContext.GetNrfUri())
	if client == nil {
		return nil, openapi.ReportError("nrf not found")
	}

	ctx, pd, err := nefContext.GetTokenCtx(models.ServiceName_NNRF_NFM, models.NrfNfManagementNfType_NRF)
	if err != nil {
		return pd, err
	}

	nfInstId := nefContext.GetNfInstID()

	request := &Nnrf_NFManagement.DeregisterNFInstanceRequest{
		NfInstanceID: &nfInstId,
	}

	_, err = client.NFInstanceIDDocumentApi.DeregisterNFInstance(ctx, request)
	if err != nil {
		switch apiErr := err.(type) {
		// API error
		case openapi.GenericOpenAPIError:
			switch errModel := apiErr.Model().(type) {
			case Nnrf_NFManagement.DeregisterNFInstanceError:
				problemDetails = &errModel.ProblemDetails
			case error:
				problemDetails = openapi.ProblemDetailsSystemFailure(errModel.Error())
			default:
				err = openapi.ReportError("openapi error")
			}
		case error:
			problemDetails = openapi.ProblemDetailsSystemFailure(apiErr.Error())
		default:
			err = openapi.ReportError("server no response")
		}
	}

	return problemDetails, err
}
