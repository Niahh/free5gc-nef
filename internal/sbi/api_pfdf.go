package sbi

import (
	"net/http"

	"github.com/free5gc/nef/internal/logger"
	"github.com/free5gc/openapi"
	"github.com/free5gc/openapi/models"
	"github.com/gin-gonic/gin"
)

func (s *Server) getPFDFRoutes() []Route {
	return []Route{
		{
			Method:  http.MethodGet,
			Pattern: "/applications",
			APIFunc: s.apiGetApplicationsPFD,
		},
		{
			Method:  http.MethodGet,
			Pattern: "/applications/:appID",
			APIFunc: s.apiGetIndividualApplicationPFD,
		},
		{
			Method:  http.MethodPost,
			Pattern: "/subscriptions",
			APIFunc: s.apiPostPFDSubscriptions,
		},
		{
			Method:  http.MethodDelete,
			Pattern: "/subscriptions/:subID",
			APIFunc: s.apiDeleteIndividualPFDSubscription,
		},
	}
}

func (s *Server) apiGetApplicationsPFD(gc *gin.Context) {
	// TODO: support URI query parameters: supported-features
	s.Processor().GetApplicationsPFD(gc, gc.QueryArray("application-ids"))
}

func (s *Server) apiGetIndividualApplicationPFD(gc *gin.Context) {
	// TODO: support URI query parameters: supported-features
	s.Processor().GetIndividualApplicationPFD(gc, gc.Param("appID"))
}

func (s *Server) apiPostPFDSubscriptions(gc *gin.Context) {
	var pfdSubsc models.PfdSubscription
	reqBody, err := gc.GetRawData()
	if err != nil {
		logger.SBILog.Errorf("Get Request Body error: %+v", err)
		gc.JSON(http.StatusInternalServerError,
			openapi.ProblemDetailsSystemFailure(err.Error()))
		return
	}

	err = openapi.Deserialize(&pfdSubsc, reqBody, "application/json")
	if err != nil {
		logger.SBILog.Errorf("Deserialize Request Body error: %+v", err)
		gc.JSON(http.StatusBadRequest,
			openapi.ProblemDetailsMalformedReqSyntax(err.Error()))
		return
	}

	s.Processor().PostPFDSubscriptions(gc, &pfdSubsc)
}

func (s *Server) apiDeleteIndividualPFDSubscription(gc *gin.Context) {
	s.Processor().DeleteIndividualPFDSubscription(gc, gc.Param("subID"))
}
