package typebot_model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Status de uma sessão de conversa com o bot.
const (
	SessionOpened = "opened"
	SessionPaused = "paused"
	SessionClosed = "closed"
)

// Typebot é a configuração de um bot para uma instância.
//
// A tabela aceita mais de uma linha por instância — é só uma FK, não custa nada
// — mas a seleção hoje pega o primeiro habilitado. Isso evita ter que migrar o
// schema se um dia for preciso rotear entre vários bots por keyword ou regex.
//
// Portado de evolution-foundation/evolution-api
// (src/api/integrations/chatbot/typebot), Apache 2.0. Lá a configuração é
// genérica para sete integrações; aqui é só Typebot, e os campos que aquela
// base tem e não usamos ficaram de fora: debounceTime, keepOpen, fallback,
// triggerType/triggerOperator e ignoreJids.
type Typebot struct {
	Id         string `json:"id" gorm:"type:uuid;primaryKey" example:"11111111-2222-3333-4444-555555555555"`
	InstanceID string `json:"instanceId" gorm:"index;not null" example:"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"`

	Enabled     bool   `json:"enabled" gorm:"default:true" example:"true"`
	Description string `json:"description" example:"Bot de atendimento"`

	// URL base do Typebot (ex.: https://viewer.exemplo.net) e o nome público do
	// fluxo, que é o {typebot} da rota /api/v1/typebots/{typebot}/startChat.
	URL     string `json:"url" gorm:"not null" example:"https://viewer.typebot.io"`
	Typebot string `json:"typebot" gorm:"not null" example:"meu-fluxo"`

	// Expire é o tempo em minutos sem interação após o qual a sessão é
	// encerrada e a próxima mensagem começa um fluxo novo. Zero desliga a
	// expiração.
	Expire int `json:"expire" gorm:"default:0" example:"0"`

	// KeywordFinish é a palavra que o contato manda para encerrar (ex.: "#sair").
	KeywordFinish string `json:"keywordFinish" example:"#sair"`

	// UnknownMessage é enviado quando o Typebot responde sem nenhum texto.
	UnknownMessage string `json:"unknownMessage" example:"Desculpe, nao entendi."`

	// DelayMessage é a pausa em milissegundos antes de cada mensagem enviada,
	// para a resposta não parecer instantânea demais.
	DelayMessage int `json:"delayMessage" gorm:"default:0" example:"0"`

	// ListeningFromMe faz o bot reagir também às mensagens enviadas pela própria
	// instância. StopBotFromMe encerra a sessão quando o operador escreve
	// manualmente na conversa — é o que permite assumir um atendimento.
	ListeningFromMe bool `json:"listeningFromMe" gorm:"default:false" example:"false"`
	StopBotFromMe   bool `json:"stopBotFromMe" gorm:"default:true" example:"true"`

	CreatedAt time.Time `json:"createdAt" gorm:"autoCreateTime" example:"2026-01-15T10:30:00Z"`
	UpdatedAt time.Time `json:"updatedAt" gorm:"autoUpdateTime" example:"2026-01-15T10:30:00Z"`
}

func (m *Typebot) BeforeCreate(tx *gorm.DB) (err error) {
	if m.Id == "" {
		m.Id = uuid.New().String()
	}
	return
}

// TypebotSession é a conversa em andamento entre um contato e um bot.
//
// SessionID guarda o identificador devolvido pelo Typebot no startChat, usado
// depois no continueChat. Ele fica num campo próprio: o projeto de origem
// concatena "{id}-{sessionId}" numa string só e recupera com split('-'), o que
// quebra se o identificador contiver hífen.
type TypebotSession struct {
	Id         string `json:"id" gorm:"type:uuid;primaryKey" example:"66666666-7777-8888-9999-000000000000"`
	InstanceID string `json:"instanceId" gorm:"index;not null" example:"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"`
	TypebotID  string `json:"typebotId" gorm:"index;not null" example:"11111111-2222-3333-4444-555555555555"`

	// RemoteJid identifica o contato (ex.: 5588999999999@s.whatsapp.net).
	RemoteJid string `json:"remoteJid" gorm:"index;not null" example:"5511999999999@s.whatsapp.net"`
	PushName  string `json:"pushName" example:"Alice Souza"`

	SessionID string `json:"sessionId" example:"clx1a2b3c4d5e6f7g8h9i0j"`
	Status    string `json:"status" gorm:"default:'opened'" example:"opened"`

	// AwaitUser indica que a última mensagem foi do bot e estamos esperando o
	// contato responder.
	AwaitUser bool `json:"awaitUser" gorm:"default:false" example:"true"`

	// Contagem para o limite por contato. Fica na sessão, e não em memória,
	// porque a sessão já é gravada a cada mensagem — o custo é praticamente
	// zero e o estado sobrevive a um restart, que é justamente quando um
	// contato em flood não deveria ganhar contador zerado.
	MsgCount    int       `json:"msgCount" gorm:"default:0" example:"3"`
	WindowStart time.Time `json:"windowStart" example:"2026-01-15T10:30:00Z"`

	// PausedReason registra por que uma pausa automática aconteceu, para que a
	// origem continue visível depois — o webhook de alerta é entregue uma vez
	// só e pode se perder.
	PausedReason string `json:"pausedReason" example:""`

	CreatedAt time.Time `json:"createdAt" gorm:"autoCreateTime" example:"2026-01-15T10:30:00Z"`
	UpdatedAt time.Time `json:"updatedAt" gorm:"autoUpdateTime" example:"2026-01-15T10:35:00Z"`
}

func (m *TypebotSession) BeforeCreate(tx *gorm.DB) (err error) {
	if m.Id == "" {
		m.Id = uuid.New().String()
	}
	return
}

// IsExpired informa se a sessão passou do tempo de inatividade configurado.
// expireMinutes igual a zero desliga a expiração.
func (m *TypebotSession) IsExpired(expireMinutes int, now time.Time) bool {
	if expireMinutes <= 0 {
		return false
	}
	return now.Sub(m.UpdatedAt) > time.Duration(expireMinutes)*time.Minute
}

// TypebotRequest é o corpo aceito na criação e na atualização de um bot.
// Os bools são ponteiros para que chaves omitidas num PUT não sejam gravadas
// como false — mesmo padrão de AdvancedSettings em instance_model.
type TypebotRequest struct {
	Enabled         *bool  `json:"enabled" example:"true"`
	Description     string `json:"description" example:"Bot de atendimento"`
	URL             string `json:"url" example:"https://viewer.typebot.io"`
	Typebot         string `json:"typebot" example:"meu-fluxo"`
	Expire          *int   `json:"expire" example:"0"`
	KeywordFinish   string `json:"keywordFinish" example:"#sair"`
	UnknownMessage  string `json:"unknownMessage" example:"Desculpe, nao entendi."`
	DelayMessage    *int   `json:"delayMessage" example:"0"`
	ListeningFromMe *bool  `json:"listeningFromMe" example:"false"`
	StopBotFromMe   *bool  `json:"stopBotFromMe" example:"true"`
}
