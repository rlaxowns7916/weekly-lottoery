package main

import (
	"log"
	"weekly-lotto/internal/config"
	"weekly-lotto/internal/domain"
	"weekly-lotto/internal/lottery"
	"weekly-lotto/internal/notify"
)

func main() {
	// 1. Load configuration from environment variables
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("❌ 설정 로드 실패: %v", err)
	}

	emailSender := notify.NewEmailSender(&cfg.Email)

	// 2. Create lottery client (auto login)
	client, err := lottery.NewClient(cfg.Credential.Username, cfg.Credential.Password)
	if err != nil {
		log.Fatalf("❌ 로그인 실패: %v", err)
	}

	log.Println("✅ 로그인 성공")

	// 3. Create 5 automatic tickets
	tickets := domain.NewAutoTickets(2)
	log.Printf("📝 자동 %d장 구매 준비", len(tickets))

	// 4. Purchase tickets
	purchased, err := client.BuyLotto645(tickets)
	if err != nil {
		log.Fatalf("❌ 구매 실패: %v", err)
	}

	// 5. Print and save purchased numbers
	log.Printf("✅ 로또 %d장 구매 완료", len(tickets))
	for _, ticket := range purchased {
		log.Printf("  슬롯 %s (%s): %v", ticket.Slot, ticket.Mode, ticket.Numbers)
	}

	// 6. sendEmail
	if err := emailSender.SendLotteryBuyMail(purchased); err != nil {
		log.Fatalf("❌ 구매 결과 이메일 전송 실패: %v", err)
	}
	log.Println("✉️  구매 결과 이메일 전송 완료")
}
