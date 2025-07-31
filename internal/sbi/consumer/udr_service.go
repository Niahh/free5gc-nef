package consumer

import (
	"fmt"
	"github.com/free5gc/nef/internal/logger"
	"github.com/free5gc/nef/internal/util"
	"github.com/free5gc/openapi"
	"github.com/free5gc/openapi/models"
	Nnrf_NFDiscovery "github.com/free5gc/openapi/nrf/NFDiscovery"
	Nudr_DataRepository "github.com/free5gc/openapi/udr/DataRepository"
	"sync"
)

type nudrService struct {
	consumer *Consumer

	mu      sync.RWMutex
	clients map[string]*Nudr_DataRepository.APIClient
}

func (s *nudrService) getDataRepositoryClient(uri string) *Nudr_DataRepository.APIClient {
	if uri == "" {
		return nil
	}

	s.mu.RLock()

	client, ok := s.clients[uri]

	if ok {
		defer s.mu.RUnlock()
		return client
	}

	configuration := Nudr_DataRepository.NewConfiguration()
	configuration.SetBasePath(uri)
	client = Nudr_DataRepository.NewAPIClient(configuration)

	s.mu.RUnlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clients[uri] = client
	return client
}

func (s *nudrService) getUdrDrUri() (string, error) {
	nefCtx := s.consumer.Context()
	udrDrUri := nefCtx.GetUdrDrUri()

	if udrDrUri == "" {

		param := &Nnrf_NFDiscovery.SearchNFInstancesRequest{}

		searchResult, err := s.consumer.SendSearchNFInstances(
			nefCtx.GetNrfUri(), models.NrfNfManagementNfType_UDR, models.NrfNfManagementNfType_NEF, param)

		if err != nil {
			logger.ConsumerLog.Errorf("NEF can not select an UDR by NRF")
			return "", err
		}

		for _, profile := range searchResult.NfInstances {
			sUri := util.SearchNFServiceUri(
				profile, models.ServiceName_NUDR_DR, models.NfServiceStatus_REGISTERED)
			if sUri != "" {
				nefCtx.SetUdrDrUri(sUri)
				return sUri, nil
			}
		}
		return udrDrUri, fmt.Errorf("nef did not find any suitable UDR after NRF discovery")
	}
	return udrDrUri, nil
}

// AppDataInfluenceDataGet Query the UDR to retrieve models.TrafficInfluData for each matching combination
// of the values of the elements of the array given in parameters.
// 3GPP TS 29.519
// 6.2.5.1 Influence Data
func (s *nudrService) AppDataInfluenceDataGet(influenceIDs []string) (
	[]models.TrafficInfluData, *models.ProblemDetails, error) {

	uri, err := s.getUdrDrUri()

	if err != nil {
		return nil, nil, err
	}

	client := s.getDataRepositoryClient(uri)

	if client == nil {
		return nil, nil, fmt.Errorf("could not initialize the DataRepository client")
	}

	param := Nudr_DataRepository.ReadInfluenceDataRequest{
		InfluenceIds: influenceIDs,
	}

	ctx, _, err := s.consumer.Context().GetTokenCtx(models.ServiceName_NUDR_DR, models.NrfNfManagementNfType_UDR)
	if err != nil {
		return nil, nil, err
	}

	influenceDataRsp, influenceDataErr := client.InfluenceDataStoreApi.ReadInfluenceData(ctx, &param)

	if influenceDataErr != nil {
		switch apiErr := influenceDataErr.(type) {
		// API error
		case openapi.GenericOpenAPIError:
			switch errorModel := apiErr.Model().(type) {
			case Nudr_DataRepository.ReadInfluenceDataError:
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

	return influenceDataRsp.TrafficInfluData, nil, nil
}

// TODO: I remove the AppDataInfluenceDataIdGet as it is the same as AppDataInfluenceDataGet, duplicated code ?

// AppDataInfluenceDataPut Stores the individual Influence Data given in parameter.
// 3GPP TS 29.519
// Table 6.2.6.3.1
func (s *nudrService) AppDataInfluenceDataPut(influenceID string,
	tiData *models.TrafficInfluData,
) (*models.TrafficInfluData, *models.ProblemDetails, error) {

	uri, err := s.getUdrDrUri()
	if err != nil {
		return nil, nil, err
	}

	client := s.getDataRepositoryClient(uri)

	if client == nil {
		return nil, nil, openapi.ReportError("could not initialize the DataRepository client")
	}

	ctx, _, err := s.consumer.Context().GetTokenCtx(models.ServiceName_NUDR_DR, models.NrfNfManagementNfType_UDR)
	if err != nil {
		return nil, nil, err
	}

	influenceDataReq := Nudr_DataRepository.CreateOrReplaceIndividualInfluenceDataRequest{
		InfluenceId:      &influenceID,
		TrafficInfluData: tiData,
	}

	influenceDataResp, errInfluenceData := client.IndividualInfluenceDataDocumentApi.
		CreateOrReplaceIndividualInfluenceData(ctx, &influenceDataReq)

	if errInfluenceData != nil {
		switch apiErr := errInfluenceData.(type) {
		// API error
		case openapi.GenericOpenAPIError:
			switch errorModel := apiErr.Model().(type) {
			case Nudr_DataRepository.CreateOrReplaceIndividualInfluenceDataError:

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

	return &influenceDataResp.TrafficInfluData, nil, nil
}

// AppDataPfdsGet Retrieve PFDs for application identifier(s) identified by query parameter(s).
// 3GPP TS 29.519
// TS 29.519 6.2.3.3.1
func (s *nudrService) AppDataPfdsGet(appIDs []string) ([]models.PfdDataForAppExt, *models.ProblemDetails, error) {
	uri, err := s.getUdrDrUri()
	if err != nil {
		return nil, nil, err
	}

	client := s.getDataRepositoryClient(uri)

	if client == nil {
		return nil, nil, openapi.ReportError("could not initialize the DataRepository client")
	}

	ctx, _, err := s.consumer.Context().GetTokenCtx(models.ServiceName_NUDR_DR, models.NrfNfManagementNfType_UDR)
	if err != nil {
		return nil, nil, err
	}

	pfdDataReq := Nudr_DataRepository.ReadPFDDataRequest{
		AppId: appIDs,
	}

	pfdDataResp, errPfdData := client.PFDDataStoreApi.ReadPFDData(ctx, &pfdDataReq)

	if errPfdData != nil {
		switch apiErr := errPfdData.(type) {
		// API error
		case openapi.GenericOpenAPIError:
			switch errorModel := apiErr.Model().(type) {
			case Nudr_DataRepository.ReadPFDDataError:
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

	return pfdDataResp.PfdDataForAppExt, nil, nil
}

// AppDataPfdsAppIdPut Creates, updates an individual PFD given an appId and the content to store into the UDR
// 3GPP TS 29.519
// 6.2.4.3.3
func (s *nudrService) AppDataPfdsAppIdPut(appID string, pfdDataForApp *models.PfdDataForAppExt) (*models.PfdDataForAppExt, *models.ProblemDetails, error) {

	uri, err := s.getUdrDrUri()
	if err != nil {
		return nil, nil, err
	}

	client := s.getDataRepositoryClient(uri)

	if client == nil {
		return nil, nil, openapi.ReportError("could not initialize the DataRepository client")
	}

	ctx, _, err := s.consumer.Context().GetTokenCtx(models.ServiceName_NUDR_DR, models.NrfNfManagementNfType_UDR)
	if err != nil {
		return nil, nil, err
	}

	individualPfdDataReq := Nudr_DataRepository.CreateOrReplaceIndividualPFDDataRequest{
		AppId:            &appID,
		PfdDataForAppExt: pfdDataForApp,
	}

	individualPfdDataRsp, errIndividualPfdData := client.IndividualPFDDataDocumentApi.CreateOrReplaceIndividualPFDData(ctx, &individualPfdDataReq)

	if errIndividualPfdData != nil {
		switch apiErr := errIndividualPfdData.(type) {
		// API error
		case openapi.GenericOpenAPIError:
			switch errorModel := apiErr.Model().(type) {
			case Nudr_DataRepository.CreateOrReplaceIndividualPFDDataError:
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

	return &individualPfdDataRsp.PfdDataForAppExt, nil, nil

}

// TS 29.519 v15.3.0 6.2.4.3.2
func (s *nudrService) AppDataPfdsAppIdDelete(appID string) (*models.ProblemDetails, error) {

	uri, err := s.getUdrDrUri()

	if err != nil {
		return nil, err
	}

	client := s.getDataRepositoryClient(uri)

	if client == nil {
		return nil, openapi.ReportError("could not initialize the DataRepository client")
	}

	ctx, _, err := s.consumer.Context().GetTokenCtx(models.ServiceName_NUDR_DR, models.NrfNfManagementNfType_UDR)
	if err != nil {
		return nil, err
	}

	DeletePdfDataReq := Nudr_DataRepository.DeleteIndividualPFDDataRequest{
		AppId: &appID,
	}

	_, errDeletePfdData := client.IndividualPFDDataDocumentApi.DeleteIndividualPFDData(ctx, &DeletePdfDataReq)

	if errDeletePfdData != nil {
		switch apiErr := errDeletePfdData.(type) {
		// API error
		case openapi.GenericOpenAPIError:
			switch errorModel := apiErr.Model().(type) {
			case Nudr_DataRepository.DeleteIndividualPFDDataError:
				return &errorModel.ProblemDetails, nil
			case error:
				return openapi.ProblemDetailsSystemFailure(errorModel.Error()), nil
			default:
				return nil, openapi.ReportError("openapi error")
			}
		case error:
			return openapi.ProblemDetailsSystemFailure(apiErr.Error()), nil
		default:
			return nil, openapi.ReportError("server no response")
		}
	}
	return nil, nil
}

// TS 29.519 v17.6.0 6.2.4.3.1
func (s *nudrService) AppDataPfdsAppIdGet(appID string) (

	*Nudr_DataRepository.ReadIndividualPFDDataResponse, *models.ProblemDetails, error) {
	uri, err := s.getUdrDrUri()
	if err != nil {
		return nil, nil, err
	}
	client := s.getDataRepositoryClient(uri)

	if client == nil {
		return nil, nil, openapi.ReportError("could not initialize the DataRepository client")
	}

	ctx, _, err := s.consumer.Context().GetTokenCtx(models.ServiceName_NUDR_DR, models.NrfNfManagementNfType_UDR)
	if err != nil {
		return nil, nil, err
	}

	pfdDataReq := Nudr_DataRepository.ReadIndividualPFDDataRequest{
		AppId: &appID,
	}

	pfdData, errPfdData := client.IndividualPFDDataDocumentApi.ReadIndividualPFDData(ctx, &pfdDataReq)

	if errPfdData != nil {
		switch apiErr := errPfdData.(type) {
		// API error
		case openapi.GenericOpenAPIError:
			switch errorModel := apiErr.Model().(type) {
			case Nudr_DataRepository.ReadIndividualPFDDataError:
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
	return pfdData, nil, nil
}

// 3GPP TS 29.519 - 6.2.6.3.2
func (s *nudrService) AppDataInfluenceDataPatch(
	influenceID string, tiSubPatch *models.TrafficInfluDataPatch,
) (*models.ProblemDetails, error) {

	uri, err := s.getUdrDrUri()
	if err != nil {
		return nil, err
	}
	client := s.getDataRepositoryClient(uri)

	ctx, _, err := s.consumer.Context().GetTokenCtx(models.ServiceName_NUDR_DR, models.NrfNfManagementNfType_UDR)
	if err != nil {
		return nil, err
	}

	tiDataReq := Nudr_DataRepository.UpdateIndividualInfluenceDataRequest{
		InfluenceId: &influenceID,
	}

	_, errTiData := client.IndividualInfluenceDataDocumentApi.UpdateIndividualInfluenceData(ctx, &tiDataReq)

	if errTiData != nil {
		switch apiErr := errTiData.(type) {
		// API error
		case openapi.GenericOpenAPIError:
			switch errorModel := apiErr.Model().(type) {
			case Nudr_DataRepository.UpdateIndividualInfluenceDataError:
				return &errorModel.ProblemDetails, nil
			case error:
				return openapi.ProblemDetailsSystemFailure(errorModel.Error()), nil
			default:
				return nil, openapi.ReportError("openapi error")
			}
		case error:
			return openapi.ProblemDetailsSystemFailure(apiErr.Error()), nil
		default:
			return nil, openapi.ReportError("server no response")
		}
	}

	return nil, nil
}

func (s *nudrService) AppDataInfluenceDataDelete(influenceID string) (*models.ProblemDetails, error) {

	uri, err := s.getUdrDrUri()
	if err != nil {
		return nil, err
	}
	client := s.getDataRepositoryClient(uri)

	if client == nil {
		return nil, openapi.ReportError("could not initialize the DataRepository client")
	}

	ctx, _, err := s.consumer.Context().GetTokenCtx(models.ServiceName_NUDR_DR, models.NrfNfManagementNfType_UDR)
	if err != nil {
		return nil, err
	}

	deleteInfluenceReq := Nudr_DataRepository.DeleteIndividualInfluenceDataRequest{
		InfluenceId: &influenceID,
	}

	_, errDeleteInfluenceData := client.IndividualInfluenceDataDocumentApi.DeleteIndividualInfluenceData(ctx, &deleteInfluenceReq)

	if errDeleteInfluenceData != nil {
		switch apiErr := errDeleteInfluenceData.(type) {
		// API error
		case openapi.GenericOpenAPIError:
			switch errorModel := apiErr.Model().(type) {
			case Nudr_DataRepository.DeleteIndividualInfluenceDataError:
				return &errorModel.ProblemDetails, nil
			case error:
				return openapi.ProblemDetailsSystemFailure(errorModel.Error()), nil
			default:
				return nil, openapi.ReportError("openapi error")
			}
		case error:
			return openapi.ProblemDetailsSystemFailure(apiErr.Error()), nil
		default:
			return nil, openapi.ReportError("server no response")
		}
	}

	return nil, nil
}
