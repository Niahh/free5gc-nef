package notifier

import (
	"context"
	"errors"
	"net/http"
	"runtime/debug"
	"strconv"
	"sync"

	nef_context "github.com/free5gc/nef/internal/context"
	"github.com/free5gc/nef/internal/logger"
	"github.com/free5gc/openapi"
	"github.com/free5gc/openapi/models"
	Nnef_PFDmanagement "github.com/free5gc/openapi/nef/PFDmanagement"
)

type PfdChangeNotifier struct {
	nefCtx              *nef_context.NefContext
	clientPfdManagement *Nnef_PFDmanagement.APIClient
	mu                  sync.RWMutex

	numPfdSubID   uint64
	appIdToSubIDs map[string]map[string]bool
	subIdToURI    map[string]string
}

type PfdNotifyContext struct {
	notifier             *PfdChangeNotifier
	appIdToNotification  map[string]models.PfdChangeNotification
	subIdToChangedAppIDs map[string][]string
}

func NewPfdChangeNotifier(_nefCtx *nef_context.NefContext) (*PfdChangeNotifier, error) {
	return &PfdChangeNotifier{
		appIdToSubIDs: make(map[string]map[string]bool),
		subIdToURI:    make(map[string]string),
		nefCtx:        _nefCtx,
	}, nil
}

func (n *PfdChangeNotifier) initPfdManagementApiClient() {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.clientPfdManagement != nil {
		return
	}

	config := Nnef_PFDmanagement.NewConfiguration()
	config.SetHTTPClient(http.DefaultClient)
	n.clientPfdManagement = Nnef_PFDmanagement.NewAPIClient(config)
}

func (n *PfdChangeNotifier) AddPfdSub(pfdSub *models.PfdSubscription) string {
	n.initPfdManagementApiClient()

	n.mu.Lock()
	defer n.mu.Unlock()

	n.numPfdSubID++
	subID := strconv.FormatUint(n.numPfdSubID, 10)
	n.subIdToURI[subID] = pfdSub.NotifyUri
	// TODO: If pfdSub.ApplicationIds is empty, it may means monitoring all appIDs
	for _, appID := range pfdSub.ApplicationIds {
		if _, exist := n.appIdToSubIDs[appID]; !exist {
			n.appIdToSubIDs[appID] = make(map[string]bool)
		}
		n.appIdToSubIDs[appID][subID] = true
	}

	return subID
}

func (n *PfdChangeNotifier) DeletePfdSub(subID string) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if _, exist := n.subIdToURI[subID]; !exist {
		return errors.New("subscription not found")
	}
	delete(n.subIdToURI, subID)
	for _, subIDs := range n.appIdToSubIDs {
		delete(subIDs, subID)
	}
	return nil
}

func (n *PfdChangeNotifier) getSubIDs(appID string) []string {
	n.mu.RLock()
	defer n.mu.RUnlock()

	subIDs := make([]string, 0, len(n.appIdToSubIDs[appID]))
	for subID := range n.appIdToSubIDs[appID] {
		subIDs = append(subIDs, subID)
	}
	return subIDs
}

func (n *PfdChangeNotifier) getSubURI(subID string) string {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.subIdToURI[subID]
}

func (n *PfdChangeNotifier) NewPfdNotifyContext() *PfdNotifyContext {
	return &PfdNotifyContext{
		notifier:             n,
		appIdToNotification:  make(map[string]models.PfdChangeNotification),
		subIdToChangedAppIDs: make(map[string][]string),
	}
}

func (nc *PfdNotifyContext) AddNotification(appID string, notif *models.PfdChangeNotification) {
	nc.appIdToNotification[appID] = *notif
	for _, subID := range nc.notifier.getSubIDs(appID) {
		nc.subIdToChangedAppIDs[subID] = append(nc.subIdToChangedAppIDs[subID], appID)
	}
}

func (nc *PfdNotifyContext) FlushNotifications() {
	for subID, appIDs := range nc.subIdToChangedAppIDs {
		pfdChangeNotifications := make([]models.PfdChangeNotification, 0, len(appIDs))
		for _, appID := range appIDs {
			pfdChangeNotifications = append(pfdChangeNotifications, nc.appIdToNotification[appID])
		}

		go func(id string) {
			defer func() {
				if p := recover(); p != nil {
					// Print stack for panic to log. Fatalf() will let program exit.
					logger.PFDManageLog.Fatalf("panic: %v\n%s", p, string(debug.Stack()))
				}
			}()

			ctx, _, errCtx := nc.notifier.nefCtx.GetTokenCtx(
				models.ServiceName_NPCF_POLICYAUTHORIZATION, models.NrfNfManagementNfType_PCF)
			if errCtx != nil {
				// TODO: Are we sure we want to force exit the application if notifications are not sent ?
				logger.PFDManageLog.Fatal(errCtx)
			}

			pfdManagementNotifyReq := Nnef_PFDmanagement.NnefPFDmanagementNotifyRequest{
				PfdChangeNotification: pfdChangeNotifications,
			}

			pd, errNotify := Notify(nc.notifier.clientPfdManagement.PFDSubscriptionsApi,
				ctx, nc.notifier.getSubURI(id), &pfdManagementNotifyReq)

			switch {
			case pd != nil:
				logger.PFDManageLog.Fatal(pd.Detail)
			case errNotify != nil:
				logger.PFDManageLog.Fatal(errNotify)
			}
		}(subID)
		// TODO: Handle the response of notification properly
	}
}

func Notify(
	subsApiService *Nnef_PFDmanagement.PFDSubscriptionsApiService,
	ctx context.Context,
	subscriberUri string,
	notifyReq *Nnef_PFDmanagement.NnefPFDmanagementNotifyRequest,
) (*models.ProblemDetails, error) {
	// TODO: Handle PfdChangeReports
	_, errNotidy := subsApiService.NnefPFDmanagementNotify(
		ctx, subscriberUri, notifyReq)

	if errNotidy != nil {
		switch apiErr := errNotidy.(type) {
		// API error
		case openapi.GenericOpenAPIError:
			switch errorModel := apiErr.Model().(type) {
			case Nnef_PFDmanagement.NnefPFDmanagementNotifyError:
				// TODO: handle the 307/308 http status code
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
