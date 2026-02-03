package services

import (
	"mooc-manus/internal/domains/models"
	"mooc-manus/internal/infra/repositories"

	"github.com/google/uuid"
)

type AppConfigDomainService interface {
	Create(models.AppConfigDO) error
	Update(models.AppConfigDO) error
	GetById(string) (models.AppConfigDO, error)
	DeleteById(string) error
	List() ([]models.AppConfigDO, error)
	CreateA2AServers(models.AppConfigDO) error
	UpdateA2AServers(do models.AppConfigDO) error
	DeleteA2AServers([]string) error
	GetA2AServers(string) ([]models.A2AServerConfigDO, error)
}

type AppConfigDomainServiceImpl struct {
	appConfigRepo     repositories.AppConfigRepository
	functionDomainSvc ToolFunctionDomainService
}

func NewAppConfigDomainService(
	appConfigRepo repositories.AppConfigRepository,
	functionDomainSvc ToolFunctionDomainService) AppConfigDomainService {
	return &AppConfigDomainServiceImpl{
		appConfigRepo:     appConfigRepo,
		functionDomainSvc: functionDomainSvc,
	}
}

func (a *AppConfigDomainServiceImpl) Create(do models.AppConfigDO) error {
	po := models.ConvertAppConfigDO2PO(do)
	return a.appConfigRepo.Create(po)
}

func (a *AppConfigDomainServiceImpl) Update(do models.AppConfigDO) error {
	po := models.ConvertAppConfigDO2PO(do)
	return a.appConfigRepo.Update(po)
}

func (a *AppConfigDomainServiceImpl) GetById(id string) (models.AppConfigDO, error) {
	po, err := a.appConfigRepo.GetById(id)
	if err != nil {
		return models.AppConfigDO{}, err
	}
	return models.ConvertAppConfigPO2DO(po), nil
}

func (a *AppConfigDomainServiceImpl) DeleteById(id string) error {
	return a.appConfigRepo.DeleteById(id)
}

func (a *AppConfigDomainServiceImpl) List() ([]models.AppConfigDO, error) {
	pos, err := a.appConfigRepo.List()
	if err != nil {
		return nil, err
	}
	var dos []models.AppConfigDO
	for _, po := range pos {
		dos = append(dos, models.ConvertAppConfigPO2DO(po))
	}
	return dos, nil
}

func (a *AppConfigDomainServiceImpl) CreateA2AServers(do models.AppConfigDO) error {
	srvCfgs := make([]models.A2AServerConfigDO, 0, len(do.A2AServerConfigs))
	srvFuncs := make([]models.A2AServerFunctionDO, 0, 0)
	for _, cfg := range do.A2AServerConfigs {
		srvCfg := models.ConvertA2AServerConfig2DO(cfg)
		srvCfg.ID = uuid.New().String()
		srvCfg.AppConfigID = do.AppConfigID
		srvCfgs = append(srvCfgs, srvCfg)
		// 创建a2a_server_function记录
		for _, funcId := range cfg.FunctionIds {
			srvFunc := models.A2AServerFunctionDO{}
			srvFunc.ID = uuid.New().String()
			srvFunc.A2AServerConfigID = srvCfg.ID
			srvFunc.FunctionID = funcId
			srvFuncs = append(srvFuncs, srvFunc)
		}
	}

	// 批量存储
	cfgPOs, err := models.ConvertA2AServerConfigDOs2POs(srvCfgs)
	if err != nil {
		return err
	}
	funcPOs := models.ConvertA2AServerFunctionDOs2POs(srvFuncs)
	return a.appConfigRepo.CreateA2AServers(cfgPOs, funcPOs)
}

func (a *AppConfigDomainServiceImpl) UpdateA2AServers(do models.AppConfigDO) error {
	srvCfgs := models.ConvertAppConfigDO2A2AServerConfigDO(do)
	srvFuncs := make([]models.A2AServerFunctionDO, 0, 0)
	for _, cfg := range do.A2AServerConfigs {
		for _, funcId := range cfg.FunctionIds {
			srvFunc := models.A2AServerFunctionDO{}
			srvFunc.ID = uuid.New().String()
			srvFunc.FunctionID = funcId
			srvFunc.A2AServerConfigID = cfg.ID
			srvFuncs = append(srvFuncs, srvFunc)
		}
	}

	// 批量更新
	srvCfgPos, err := models.ConvertA2AServerConfigDOs2POs(srvCfgs)
	if err != nil {
		return err
	}
	srvFuncPos := models.ConvertA2AServerFunctionDOs2POs(srvFuncs)
	return a.appConfigRepo.UpdateA2AServers(srvCfgPos, srvFuncPos)
}

func (a *AppConfigDomainServiceImpl) DeleteA2AServers(srvCfgIds []string) error {
	return a.appConfigRepo.DeleteA2AServers(srvCfgIds)
}

func (a *AppConfigDomainServiceImpl) GetA2AServers(appConfigId string) ([]models.A2AServerConfigDO, error) {
	cfgPOs, err := a.appConfigRepo.GetA2AServerConfigsByAppConfigId(appConfigId)
	if err != nil {
		return nil, err
	}
	srvCfgs, err := models.ConvertA2AServerConfigPOs2DOs(cfgPOs)
	if err != nil {
		return nil, err
	}
	for i, cfg := range srvCfgs {
		srvFuncs, err := a.appConfigRepo.GetA2AServerFunctionsByServerConfigIds([]string{cfg.ID})
		if err != nil {
			return nil, err
		}
		funcIds := make([]string, 0, len(srvFuncs))
		for _, srvFunc := range srvFuncs {
			funcIds = append(funcIds, srvFunc.FunctionID)
		}
		funcs, err := a.functionDomainSvc.GetByIds(funcIds)
		if err != nil {
			return nil, err
		}
		srvCfgs[i].Skills = models.ConvertToolFunctions2AgentSkills(funcs)
	}

	return srvCfgs, nil
}
