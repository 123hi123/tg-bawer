package bot

import (
	"fmt"
	"log"

	"tg-bawer/grok"
)

func (b *Bot) isAdminUser(userID int64) bool {
	return b.adminUserID != 0 && userID == b.adminUserID
}

func (b *Bot) userHasPrivateService(userID int64) bool {
	services, err := b.db.GetUserServices(userID)
	if err != nil {
		log.Printf("讀取使用者私人服務失敗 (userID=%d): %v", userID, err)
		return false
	}

	for _, service := range services {
		if !service.IsPublic {
			return true
		}
	}
	return false
}

func privateServiceBucket(serviceType string) string {
	if serviceType == grok.ServiceTypeGrok {
		return "grok"
	}
	return "google"
}

func (b *Bot) canAddPrivateService(userID int64, serviceType string) (bool, error) {
	if b.isAdminUser(userID) {
		return true, nil
	}

	services, err := b.db.GetUserServices(userID)
	if err != nil {
		return false, err
	}

	targetBucket := privateServiceBucket(serviceType)
	for _, service := range services {
		if service.IsPublic {
			continue
		}
		if privateServiceBucket(service.Type) == targetBucket {
			return false, nil
		}
	}
	return true, nil
}

func (b *Bot) ensurePrivateServiceQuota(userID int64, serviceType string) error {
	ok, err := b.canAddPrivateService(userID, serviceType)
	if err != nil {
		return err
	}
	if ok {
		return nil
	}

	switch privateServiceBucket(serviceType) {
	case "grok":
		return fmt.Errorf("一般使用者最多只能保有 1 條私人 Grok 服務")
	default:
		return fmt.Errorf("一般使用者最多只能保有 1 條私人 Google 服務")
	}
}

func (b *Bot) shouldApplyPublicServiceRateLimit(userID int64) bool {
	if b.isAdminUser(userID) {
		return false
	}
	if b.userHasPrivateService(userID) {
		return false
	}
	return true
}

func (b *Bot) hasAvailableService(userID int64) bool {
	services, err := b.resolveAllServiceConfigs(userID)
	if err == nil && len(services) > 0 {
		return true
	}
	return b.resolveGrokClient(userID) != nil
}
