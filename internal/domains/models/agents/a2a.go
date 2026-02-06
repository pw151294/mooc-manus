package agents

import (
	"mooc-manus/internal/domains/models"

	"github.com/a2aproject/a2a-go/a2a"
)

const (
	defaultPreferredTransport = a2a.TransportProtocolJSONRPC
	defaultDefaultInputMode   = "text"
	defaultDefaultOutputMode  = "text"
	defaultStreaming          = true
)

func ConvertA2AServerConfig2AgentCard(srvCfg models.A2AServerConfigDO) *a2a.AgentCard {
	agentCard := &a2a.AgentCard{}
	agentCard.Name = srvCfg.Name
	agentCard.Description = srvCfg.Description
	agentCard.URL = srvCfg.BaseURL + "/invoke"
	agentCard.PreferredTransport = defaultPreferredTransport
	agentCard.DefaultInputModes = []string{defaultDefaultInputMode}
	agentCard.DefaultOutputModes = []string{defaultDefaultOutputMode}
	agentCard.Capabilities = a2a.AgentCapabilities{Streaming: defaultStreaming}
	agentCard.Skills = ConvertAgentSkills2A2AAgentSkills(srvCfg.Skills)

	return agentCard
}

func ConvertAgentSkill2A2AAgentSkill(skill models.AgentSkill) a2a.AgentSkill {
	a2aSKill := a2a.AgentSkill{}
	a2aSKill.ID = skill.ID
	a2aSKill.Name = skill.Name
	a2aSKill.Description = skill.Description
	a2aSKill.Tags = skill.Tags
	a2aSKill.Examples = skill.Examples
	return a2aSKill
}

func ConvertAgentSkills2A2AAgentSkills(skills []models.AgentSkill) []a2a.AgentSkill {
	a2aSKills := make([]a2a.AgentSkill, 0, len(skills))
	for _, skill := range skills {
		a2aSKills = append(a2aSKills, ConvertAgentSkill2A2AAgentSkill(skill))
	}
	return a2aSKills
}
