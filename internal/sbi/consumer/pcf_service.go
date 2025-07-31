package consumer

import (
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/free5gc/nef/internal/logger"
	"github.com/free5gc/nef/internal/util"
	"github.com/free5gc/openapi"
	"github.com/free5gc/openapi/models"
	Nnrf_NFDiscovery "github.com/free5gc/openapi/nrf/NFDiscovery"
	Npcf_PolicyAuthorization "github.com/free5gc/openapi/pcf/PolicyAuthorization"
)

type npcfService struct {
	consumer *Consumer

	mu      sync.RWMutex
	clients map[string]*Npcf_PolicyAuthorization.APIClient
}

func (s *npcfService) getPolicyAuthorizationClient(uri string) *Npcf_PolicyAuthorization.APIClient {
	if uri == "" {
		return nil
	}

	s.mu.RLock()
	client, ok := s.clients[uri]
	if ok {
		defer s.mu.RUnlock()
		return client
	}

	configuration := Npcf_PolicyAuthorization.NewConfiguration()
	configuration.SetBasePath(uri)
	configuration.SetHTTPClient(http.DefaultClient)
	cli := Npcf_PolicyAuthorization.NewAPIClient(configuration)

	s.mu.RUnlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clients[uri] = cli
	return cli
}

func (s *npcfService) getPcfPolicyAuthUri() (string, error) {
	nefCtx := s.consumer.Context()
	pcfPaUri := nefCtx.GetPcfPaUri()

	if pcfPaUri == "" {
		param := &Nnrf_NFDiscovery.SearchNFInstancesRequest{
			ServiceNames: []models.ServiceName{
				models.ServiceName_NPCF_POLICYAUTHORIZATION,
			},
		}

		searchResult, err := s.consumer.SendSearchNFInstances(
			nefCtx.GetNrfUri(), models.NrfNfManagementNfType_PCF, models.NrfNfManagementNfType_NEF, param)
		if err != nil {
			logger.ConsumerLog.Errorf("NEF can not select a PCF by NRF")
			return "", err
		}

		for _, profile := range searchResult.NfInstances {
			sUri := util.SearchNFServiceUri(
				profile, models.ServiceName_NPCF_POLICYAUTHORIZATION, models.NfServiceStatus_REGISTERED)
			if sUri != "" {
				nefCtx.SetPcfPaUri(sUri)
				return sUri, nil
			}
		}
		return pcfPaUri, fmt.Errorf("nef did not find any suitable PCF after NRF discovery")
	}
	return pcfPaUri, nil
}

func (s *npcfService) GetAppSession(appSessionId string) (
	*models.AppSessionContext, *models.ProblemDetails, error,
) {
	uri, err := s.getPcfPolicyAuthUri()
	if err != nil {
		return nil, nil, err
	}

	client := s.getPolicyAuthorizationClient(uri)

	if client == nil {
		return nil, nil, openapi.ReportError("could not initialize the PolicyAuthorization client")
	}

	ctx, _, err := s.consumer.Context().GetTokenCtx(
		models.ServiceName_NPCF_POLICYAUTHORIZATION, models.NrfNfManagementNfType_PCF)
	if err != nil {
		return nil, nil, err
	}

	appSessionsRequest := Npcf_PolicyAuthorization.GetAppSessionRequest{
		AppSessionId: &appSessionId,
	}

	getAppSessionRsp, errGetAppSessionRsp := client.IndividualApplicationSessionContextDocumentApi.
		GetAppSession(ctx, &appSessionsRequest)

	if errGetAppSessionRsp != nil {
		switch apiErr := errGetAppSessionRsp.(type) {
		// API error
		case openapi.GenericOpenAPIError:
			switch errorModel := apiErr.Model().(type) {
			case Npcf_PolicyAuthorization.GetAppSessionError:
				// TODO: handle the 307/308 http status code
				return nil, &errorModel.ProblemDetails, nil
			case error:
				return nil, openapi.ProblemDetailsSystemFailure(errorModel.Error()), nil
			default:
				return nil, nil, openapi.ReportError("openapi error")
			}
		case error:
			return nil, openapi.ProblemDetailsSystemFailure(apiErr.Error()), nil
		default:
			return nil, nil, openapi.ReportError("server no response")
		}
	}

	return &getAppSessionRsp.AppSessionContext, nil, nil
}

// PostAppSessions Creates a models.AppSessionContext in the NPCF_policyAuthorization service.
// 3GPP TS 29.514 release 17 version 17.6.0
// Table 5.3.2.3.1
func (s *npcfService) PostAppSessions(asc *models.AppSessionContext) (string, *models.ProblemDetails, error) {
	uri, err := s.getPcfPolicyAuthUri()
	if err != nil {
		return "", nil, err
	}

	client := s.getPolicyAuthorizationClient(uri)

	if client == nil {
		return "", nil, openapi.ReportError("could not initialize the PolicyAuthorization client")
	}

	ctx, _, err := s.consumer.Context().GetTokenCtx(
		models.ServiceName_NPCF_POLICYAUTHORIZATION, models.NrfNfManagementNfType_PCF)
	if err != nil {
		return "", nil, err
	}

	appSessionsRequest := Npcf_PolicyAuthorization.PostAppSessionsRequest{
		AppSessionContext: asc,
	}

	postAppSessionsRsp, errPostAppSessionRsp := client.ApplicationSessionsCollectionApi.
		PostAppSessions(ctx, &appSessionsRequest)

	if errPostAppSessionRsp != nil {
		switch apiErr := errPostAppSessionRsp.(type) {
		// API error
		case openapi.GenericOpenAPIError:
			switch errorModel := apiErr.Model().(type) {
			case Npcf_PolicyAuthorization.PostAppSessionsError:
				return "", &errorModel.ProblemDetails, nil
			case error:
				return "", openapi.ProblemDetailsSystemFailure(errorModel.Error()), nil
			default:
				return "", nil, openapi.ReportError("openapi error")
			}
		case error:
			return "", openapi.ProblemDetailsSystemFailure(apiErr.Error()), nil
		default:
			return "", nil, openapi.ReportError("server no response")
		}
	}

	var sessId string

	if postAppSessionsRsp != nil {
		sessId = getAppSessIDFromRspLocationHeader(postAppSessionsRsp.Location)
	}

	return sessId, nil, nil
}

// PutAppSession Updates a models.AppSessionContext and returns its representation.
func (s *npcfService) PutAppSession(
	appSessionId string,
	ascUpdateData *models.AppSessionContextUpdateData,
) (*models.AppSessionContext, *models.ProblemDetails, error) {
	uri, err := s.getPcfPolicyAuthUri()
	if err != nil {
		return nil, nil, err
	}

	client := s.getPolicyAuthorizationClient(uri)

	if client == nil {
		return nil, nil, openapi.ReportError("could not initialize the PolicyAuthorization client")
	}

	ctx, _, err := s.consumer.Context().GetTokenCtx(
		models.ServiceName_NPCF_POLICYAUTHORIZATION, models.NrfNfManagementNfType_PCF)
	if err != nil {
		return nil, nil, err
	}

	appSessionCtx, pbAppSessionCtx, errAppSessionCtx := s.GetAppSession(appSessionId)

	var modifiedAppSessionCtx models.AppSessionContext

	switch {
	case pbAppSessionCtx != nil:
		return nil, pbAppSessionCtx, nil
	case errAppSessionCtx != nil:
		return nil, nil, errAppSessionCtx
	case appSessionCtx != nil:
		// If we retrieved the appSession, we patch it.

		appSessionCtxUpdateDataPatch := models.AppSessionContextUpdateDataPatch{AscReqData: ascUpdateData}

		modAppSessionReq := Npcf_PolicyAuthorization.ModAppSessionRequest{
			AppSessionId:                     &appSessionId,
			AppSessionContextUpdateDataPatch: &appSessionCtxUpdateDataPatch,
		}

		modAppSessionRsp, errModAppSessionRsp := client.IndividualApplicationSessionContextDocumentApi.ModAppSession(
			ctx, &modAppSessionReq)

		if errModAppSessionRsp != nil {
			switch apiErr := errModAppSessionRsp.(type) {
			// API error
			case openapi.GenericOpenAPIError:
				switch errorModel := apiErr.Model().(type) {
				case Npcf_PolicyAuthorization.ModAppSessionError:
					// TODO: handle the 307/308 http status code
					return nil, &errorModel.ProblemDetails, nil
				case error:
					return nil, openapi.ProblemDetailsSystemFailure(errorModel.Error()), nil
				default:
					return nil, nil, openapi.ReportError("openapi error")
				}
			case error:
				return nil, openapi.ProblemDetailsSystemFailure(apiErr.Error()), nil
			default:
				return nil, nil, openapi.ReportError("server no response")
			}
		}

		modifiedAppSessionCtx = modAppSessionRsp.AppSessionContext
		logger.ConsumerLog.Debugf("PatchAppSessions RspData: %+v", modifiedAppSessionCtx)
	}

	return &modifiedAppSessionCtx, nil, nil
}

// PatchAppSession Updates a models.AppSessionContext and returns its representation.
// 3GPP TS 29.514 release 17 version 17.6.0
// Table 5.3.3.3.2
func (s *npcfService) PatchAppSession(appSessionId string,
	ascUpdateData *models.AppSessionContextUpdateData,
) (*models.AppSessionContext, *models.ProblemDetails, error) {
	uri, err := s.getPcfPolicyAuthUri()
	if err != nil {
		return nil, nil, err
	}

	client := s.getPolicyAuthorizationClient(uri)

	if client == nil {
		return nil, nil, openapi.ReportError("could not initialize the PolicyAuthorization client")
	}

	ctx, _, err := s.consumer.Context().GetTokenCtx(
		models.ServiceName_NPCF_POLICYAUTHORIZATION, models.NrfNfManagementNfType_PCF)
	if err != nil {
		return nil, nil, err
	}

	appSessionCtxUpdateDataPatch := models.AppSessionContextUpdateDataPatch{AscReqData: ascUpdateData}

	modAppSessionReq := Npcf_PolicyAuthorization.ModAppSessionRequest{
		AppSessionId:                     &appSessionId,
		AppSessionContextUpdateDataPatch: &appSessionCtxUpdateDataPatch,
	}

	modAppSessionRsp, errModAppSessionRsp := client.IndividualApplicationSessionContextDocumentApi.ModAppSession(
		ctx, &modAppSessionReq)

	if errModAppSessionRsp != nil {
		switch apiErr := errModAppSessionRsp.(type) {
		// API error
		case openapi.GenericOpenAPIError:
			switch errorModel := apiErr.Model().(type) {
			case Npcf_PolicyAuthorization.ModAppSessionError:
				// TODO: handle the 307/308 http status code
				return nil, &errorModel.ProblemDetails, nil
			case error:
				return nil, openapi.ProblemDetailsSystemFailure(errorModel.Error()), nil
			default:
				return nil, nil, openapi.ReportError("openapi error")
			}
		case error:
			return nil, openapi.ProblemDetailsSystemFailure(apiErr.Error()), nil
		default:
			return nil, nil, openapi.ReportError("server no response")
		}
	}

	logger.ConsumerLog.Debugf("PatchAppSessions RspData: %+v", modAppSessionRsp.AppSessionContext)

	return &modAppSessionRsp.AppSessionContext, nil, nil
}

// DeleteAppSession Sends out a deleteAppSession API request to the PCF and returns either a status code,
// a problemDetails or an error format.
// 3GPP TS 29.514 Release 17 version 17.6.0
// Table 5.3.1-1
func (s *npcfService) DeleteAppSession(appSessionId string) (int, *models.ProblemDetails, error) {
	uri, err := s.getPcfPolicyAuthUri()
	if err != nil {
		return 0, nil, err
	}

	client := s.getPolicyAuthorizationClient(uri)

	if client == nil {
		return 0, nil, openapi.ReportError("could not initialize the PolicyAuthorization client")
	}

	ctx, _, err := s.consumer.Context().GetTokenCtx(
		models.ServiceName_NPCF_POLICYAUTHORIZATION, models.NrfNfManagementNfType_PCF)
	if err != nil {
		return 0, nil, err
	}

	deleteAppSessionReq := Npcf_PolicyAuthorization.DeleteAppSessionRequest{
		AppSessionId: &appSessionId,
		// Parameter is optional, to change when the PCF will handle it.
		PcfPolicyAuthorizationEventsSubscReqData: nil,
	}

	// Here the response do not have any interest as we do not return
	_, errDeleteAppSessRsp := client.IndividualApplicationSessionContextDocumentApi.DeleteAppSession(
		ctx, &deleteAppSessionReq)

	if errDeleteAppSessRsp != nil {
		var problemDetails *models.ProblemDetails
		switch apiErr := errDeleteAppSessRsp.(type) {
		// API error
		case openapi.GenericOpenAPIError:
			switch errorModel := apiErr.Model().(type) {
			case Npcf_PolicyAuthorization.DeleteAppSessionError:
				// TODO: handle the 307/308 http status code
				problemDetails = &errorModel.ProblemDetails
				return int(problemDetails.Status), problemDetails, nil
			case error:
				problemDetails = openapi.ProblemDetailsSystemFailure(errorModel.Error())
				return int(problemDetails.Status), problemDetails, nil
			default:
				return 0, nil, openapi.ReportError("openapi error")
			}
		case error:
			problemDetails = openapi.ProblemDetailsSystemFailure(apiErr.Error())
			return int(problemDetails.Status), problemDetails, nil
		default:
			return 0, nil, openapi.ReportError("server no response")
		}
	}

	// As per 5.4.1.3.3.5-3, we return StatusNoContent
	return http.StatusNoContent, nil, nil
}

func getAppSessIDFromRspLocationHeader(loc string) string {
	appSessID := ""
	if strings.Contains(loc, "http") {
		index := strings.LastIndex(loc, "/")
		appSessID = loc[index+1:]
	}
	logger.ConsumerLog.Infof("appSessID=%q", appSessID)
	return appSessID
}
