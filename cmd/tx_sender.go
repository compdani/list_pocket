package main

import (
	"github.com/compdani/list_pocket/internal/txemail"
)

func (a *App) newTransactionalSender() *txemail.Sender {
	return &txemail.Sender{
		Core:             a.core,
		Manager:          a.manager,
		DefaultFromEmail: a.cfg.FromEmail,
		ResolveFromEmail: a.defaultCampaignFromEmail,
		Log:              a.log,
	}
}
