package restHandler

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/devtron-labs/devtron/api/restHandler/common"
	"github.com/devtron-labs/devtron/client/argocdServer/application"
	"github.com/devtron-labs/devtron/client/gitSensor"
	"github.com/devtron-labs/devtron/internal/sql/repository/pipelineConfig"
	"github.com/devtron-labs/devtron/internal/sql/repository/security"
	"github.com/devtron-labs/devtron/pkg/appClone"
	"github.com/devtron-labs/devtron/pkg/appWorkflow"
	"github.com/devtron-labs/devtron/pkg/chart"
	request "github.com/devtron-labs/devtron/pkg/cluster"
	"github.com/devtron-labs/devtron/pkg/pipeline"
	security2 "github.com/devtron-labs/devtron/pkg/security"
	"github.com/devtron-labs/devtron/pkg/team"
	"github.com/devtron-labs/devtron/pkg/user"
	"github.com/devtron-labs/devtron/pkg/user/casbin"
	"github.com/devtron-labs/devtron/util/argo"
	"github.com/devtron-labs/devtron/util/rbac"
	"github.com/gorilla/mux"
	"go.uber.org/zap"
	"gopkg.in/go-playground/validator.v9"
	"net/http"
)

type BulkUpdateRestHandler interface {
	FindBulkUpdateReadme(w http.ResponseWriter, r *http.Request)
	GetImpactedAppsName(w http.ResponseWriter, r *http.Request)
	BulkUpdate(w http.ResponseWriter, r *http.Request)

	BulkHibernate(w http.ResponseWriter, r *http.Request)
	BulkUnHibernate(w http.ResponseWriter, r *http.Request)
	BulkDeploy(w http.ResponseWriter, r *http.Request)
}
type BulkUpdateRestHandlerImpl struct {
	pipelineBuilder         pipeline.PipelineBuilder
	ciPipelineRepository    pipelineConfig.CiPipelineRepository
	ciHandler               pipeline.CiHandler
	logger                  *zap.SugaredLogger
	bulkUpdateService       pipeline.BulkUpdateService
	chartService            chart.ChartService
	propertiesConfigService pipeline.PropertiesConfigService
	dbMigrationService      pipeline.DbMigrationService
	application             application.ServiceClient
	userAuthService         user.UserService
	validator               *validator.Validate
	teamService             team.TeamService
	enforcer                casbin.Enforcer
	gitSensorClient         gitSensor.GitSensorClient
	pipelineRepository      pipelineConfig.PipelineRepository
	appWorkflowService      appWorkflow.AppWorkflowService
	enforcerUtil            rbac.EnforcerUtil
	envService              request.EnvironmentService
	gitRegistryConfig       pipeline.GitRegistryConfig
	dockerRegistryConfig    pipeline.DockerRegistryConfig
	cdHandelr               pipeline.CdHandler
	appCloneService         appClone.AppCloneService
	materialRepository      pipelineConfig.MaterialRepository
	policyService           security2.PolicyService
	scanResultRepository    security.ImageScanResultRepository
	argoUserService         argo.ArgoUserService
}

func NewBulkUpdateRestHandlerImpl(pipelineBuilder pipeline.PipelineBuilder, logger *zap.SugaredLogger,
	bulkUpdateService pipeline.BulkUpdateService,
	chartService chart.ChartService,
	propertiesConfigService pipeline.PropertiesConfigService,
	dbMigrationService pipeline.DbMigrationService,
	application application.ServiceClient,
	userAuthService user.UserService,
	teamService team.TeamService,
	enforcer casbin.Enforcer,
	ciHandler pipeline.CiHandler,
	validator *validator.Validate,
	gitSensorClient gitSensor.GitSensorClient,
	ciPipelineRepository pipelineConfig.CiPipelineRepository, pipelineRepository pipelineConfig.PipelineRepository,
	enforcerUtil rbac.EnforcerUtil, envService request.EnvironmentService,
	gitRegistryConfig pipeline.GitRegistryConfig, dockerRegistryConfig pipeline.DockerRegistryConfig,
	cdHandelr pipeline.CdHandler,
	appCloneService appClone.AppCloneService,
	appWorkflowService appWorkflow.AppWorkflowService,
	materialRepository pipelineConfig.MaterialRepository, policyService security2.PolicyService,
	scanResultRepository security.ImageScanResultRepository,
	argoUserService argo.ArgoUserService) *BulkUpdateRestHandlerImpl {
	return &BulkUpdateRestHandlerImpl{
		pipelineBuilder:         pipelineBuilder,
		logger:                  logger,
		bulkUpdateService:       bulkUpdateService,
		chartService:            chartService,
		propertiesConfigService: propertiesConfigService,
		dbMigrationService:      dbMigrationService,
		application:             application,
		userAuthService:         userAuthService,
		validator:               validator,
		teamService:             teamService,
		enforcer:                enforcer,
		ciHandler:               ciHandler,
		gitSensorClient:         gitSensorClient,
		ciPipelineRepository:    ciPipelineRepository,
		pipelineRepository:      pipelineRepository,
		enforcerUtil:            enforcerUtil,
		envService:              envService,
		gitRegistryConfig:       gitRegistryConfig,
		dockerRegistryConfig:    dockerRegistryConfig,
		cdHandelr:               cdHandelr,
		appCloneService:         appCloneService,
		appWorkflowService:      appWorkflowService,
		materialRepository:      materialRepository,
		policyService:           policyService,
		scanResultRepository:    scanResultRepository,
		argoUserService:         argoUserService,
	}
}

func (handler BulkUpdateRestHandlerImpl) FindBulkUpdateReadme(w http.ResponseWriter, r *http.Request) {
	var operation string
	vars := mux.Vars(r)
	apiVersion := vars["apiVersion"]
	kind := vars["kind"]
	operation = fmt.Sprintf("%s/%s", apiVersion, kind)
	response, err := handler.bulkUpdateService.FindBulkUpdateReadme(operation)
	if err != nil {
		common.WriteJsonResp(w, err, nil, http.StatusInternalServerError)
		return
	}
	//auth free, only login required
	var responseArr []*pipeline.BulkUpdateSeeExampleResponse
	responseArr = append(responseArr, response)
	common.WriteJsonResp(w, nil, responseArr, http.StatusOK)
}

func (handler BulkUpdateRestHandlerImpl) CheckAuthForImpactedObjects(AppId int, EnvId int, appResourceObjects map[int]string, envResourceObjects map[string]string, token string) bool {
	resourceName := appResourceObjects[AppId]
	if ok := handler.enforcer.Enforce(token, casbin.ResourceApplications, casbin.ActionGet, resourceName); !ok {
		return ok
	}
	if EnvId > 0 {
		key := fmt.Sprintf("%d-%d", EnvId, AppId)
		envResourceName := envResourceObjects[key]
		if ok := handler.enforcer.Enforce(token, casbin.ResourceEnvironment, casbin.ActionGet, envResourceName); !ok {
			return ok
		}
	}
	return true

}
func (handler BulkUpdateRestHandlerImpl) GetImpactedAppsName(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	var script pipeline.BulkUpdateScript
	err := decoder.Decode(&script)
	if err != nil {
		common.WriteJsonResp(w, err, nil, http.StatusBadRequest)
		return
	}
	err = handler.validator.Struct(script)
	if err != nil {
		handler.logger.Errorw("validation err, Script", "err", err, "BulkUpdateScript", script)
		common.WriteJsonResp(w, err, nil, http.StatusBadRequest)
		return
	}
	token := r.Header.Get("token")
	impactedApps, err := handler.bulkUpdateService.GetBulkAppName(script.Spec)
	if err != nil {
		common.WriteJsonResp(w, err, nil, http.StatusInternalServerError)
		return
	}
	appResourceObjects, envResourceObjects := handler.enforcerUtil.GetRbacObjectsForAllAppsAndEnvironments()
	for _, deploymentTemplateImpactedApp := range impactedApps.DeploymentTemplate {
		ok := handler.CheckAuthForImpactedObjects(deploymentTemplateImpactedApp.AppId, deploymentTemplateImpactedApp.EnvId, appResourceObjects, envResourceObjects, token)
		if !ok {
			common.WriteJsonResp(w, fmt.Errorf("unauthorized user"), "Unauthorized User", http.StatusForbidden)
		}
	}
	for _, configMapImpactedApp := range impactedApps.ConfigMap {
		ok := handler.CheckAuthForImpactedObjects(configMapImpactedApp.AppId, configMapImpactedApp.EnvId, appResourceObjects, envResourceObjects, token)
		if !ok {
			common.WriteJsonResp(w, fmt.Errorf("unauthorized user"), "Unauthorized User", http.StatusForbidden)
		}
	}
	for _, secretImpactedApp := range impactedApps.Secret {
		ok := handler.CheckAuthForImpactedObjects(secretImpactedApp.AppId, secretImpactedApp.EnvId, appResourceObjects, envResourceObjects, token)
		if !ok {
			common.WriteJsonResp(w, fmt.Errorf("unauthorized user"), "Unauthorized User", http.StatusForbidden)
		}
	}
	common.WriteJsonResp(w, err, impactedApps, http.StatusOK)
}
func (handler BulkUpdateRestHandlerImpl) CheckAuthForBulkUpdate(AppId int, EnvId int, AppName string, rbacObjects map[int]string, token string) bool {
	resourceName := rbacObjects[AppId]
	if ok := handler.enforcer.Enforce(token, casbin.ResourceApplications, casbin.ActionUpdate, resourceName); !ok {
		return ok
	}
	if EnvId > 0 {
		resourceName := handler.enforcerUtil.GetAppRBACByAppNameAndEnvId(AppName, EnvId)
		if ok := handler.enforcer.Enforce(token, casbin.ResourceEnvironment, casbin.ActionUpdate, resourceName); !ok {
			return ok
		}
	}
	return true

}
func (handler BulkUpdateRestHandlerImpl) BulkUpdate(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	var script pipeline.BulkUpdateScript
	err := decoder.Decode(&script)
	if err != nil {
		common.WriteJsonResp(w, err, nil, http.StatusBadRequest)
		return
	}
	err = handler.validator.Struct(script)
	if err != nil {
		handler.logger.Errorw("validation err, Script", "err", err, "BulkUpdateScript", script)
		common.WriteJsonResp(w, err, nil, http.StatusBadRequest)
		return
	}
	token := r.Header.Get("token")
	impactedApps, err := handler.bulkUpdateService.GetBulkAppName(script.Spec)
	if err != nil {
		common.WriteJsonResp(w, err, nil, http.StatusInternalServerError)
		return
	}
	rbacObjects := handler.enforcerUtil.GetRbacObjectsForAllApps()
	for _, deploymentTemplateImpactedApp := range impactedApps.DeploymentTemplate {
		ok := handler.CheckAuthForBulkUpdate(deploymentTemplateImpactedApp.AppId, deploymentTemplateImpactedApp.EnvId, deploymentTemplateImpactedApp.AppName, rbacObjects, token)
		if !ok {
			common.WriteJsonResp(w, fmt.Errorf("unauthorized user"), "Unauthorized User", http.StatusForbidden)
		}
	}
	for _, configMapImpactedApp := range impactedApps.ConfigMap {
		ok := handler.CheckAuthForBulkUpdate(configMapImpactedApp.AppId, configMapImpactedApp.EnvId, configMapImpactedApp.AppName, rbacObjects, token)
		if !ok {
			common.WriteJsonResp(w, fmt.Errorf("unauthorized user"), "Unauthorized User", http.StatusForbidden)
		}
	}
	for _, secretImpactedApp := range impactedApps.Secret {
		ok := handler.CheckAuthForBulkUpdate(secretImpactedApp.AppId, secretImpactedApp.EnvId, secretImpactedApp.AppName, rbacObjects, token)
		if !ok {
			common.WriteJsonResp(w, fmt.Errorf("unauthorized user"), "Unauthorized User", http.StatusForbidden)
		}
	}

	response := handler.bulkUpdateService.BulkUpdate(script.Spec)
	common.WriteJsonResp(w, nil, response, http.StatusOK)
}

func (handler BulkUpdateRestHandlerImpl) BulkHibernate(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	var request pipeline.BulkApplicationForEnvironmentPayload
	err := decoder.Decode(&request)
	if err != nil {
		common.WriteJsonResp(w, err, nil, http.StatusBadRequest)
		return
	}
	err = handler.validator.Struct(request)
	if err != nil {
		handler.logger.Errorw("validation err", "err", err, "request", request)
		common.WriteJsonResp(w, err, nil, http.StatusBadRequest)
		return
	}

	acdToken, err := handler.argoUserService.GetLatestDevtronArgoCdUserToken()
	if err != nil {
		handler.logger.Errorw("error in getting acd token", "err", err)
		common.WriteJsonResp(w, err, nil, http.StatusInternalServerError)
		return
	}
	ctx := context.WithValue(r.Context(), "token", acdToken)

	response, err := handler.bulkUpdateService.BulkHibernate(&request, ctx)
	common.WriteJsonResp(w, nil, response, http.StatusOK)
}

func (handler BulkUpdateRestHandlerImpl) BulkUnHibernate(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	var request pipeline.BulkApplicationForEnvironmentPayload
	err := decoder.Decode(&request)
	if err != nil {
		common.WriteJsonResp(w, err, nil, http.StatusBadRequest)
		return
	}
	err = handler.validator.Struct(request)
	if err != nil {
		handler.logger.Errorw("validation err", "err", err, "request", request)
		common.WriteJsonResp(w, err, nil, http.StatusBadRequest)
		return
	}

	acdToken, err := handler.argoUserService.GetLatestDevtronArgoCdUserToken()
	if err != nil {
		handler.logger.Errorw("error in getting acd token", "err", err)
		common.WriteJsonResp(w, err, nil, http.StatusInternalServerError)
		return
	}
	ctx := context.WithValue(r.Context(), "token", acdToken)
	response, err := handler.bulkUpdateService.BulkUnHibernate(&request, ctx)
	common.WriteJsonResp(w, nil, response, http.StatusOK)
}

func (handler BulkUpdateRestHandlerImpl) BulkDeploy(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	var request pipeline.BulkApplicationForEnvironmentPayload
	err := decoder.Decode(&request)
	if err != nil {
		common.WriteJsonResp(w, err, nil, http.StatusBadRequest)
		return
	}
	err = handler.validator.Struct(request)
	if err != nil {
		handler.logger.Errorw("validation err", "err", err, "request", request)
		common.WriteJsonResp(w, err, nil, http.StatusBadRequest)
		return
	}
	response, err := handler.bulkUpdateService.BulkDeploy(&request)
	common.WriteJsonResp(w, nil, response, http.StatusOK)
}
