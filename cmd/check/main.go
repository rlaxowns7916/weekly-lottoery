package main

import (
	"log"
	"weekly-lotto/internal/config"
	"weekly-lotto/internal/domain"
	"weekly-lotto/internal/lottery"
	"weekly-lotto/internal/notify"
)

const purchaseHistoryDays = 7

func main() {
	// 1. Load configuration
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
	// 3. Get winning numbers
	winning, err := client.GetWinningNumbers()
	if err != nil {
		log.Fatalf("❌ 당첨 번호 조회 실패: %v", err)
	}

	// 4. Load purchased numbers from lottery purchase history
	purchases, err := client.GetRecentPurchases(purchaseHistoryDays)
	if err != nil {
		log.Fatalf("❌ 구매 내역 조회 실패: %v", err)
	}

	var purchased []lottery.PurchasedTicket
	for _, purchase := range purchases {
		if purchase.Round == winning.Round {
			purchased = append(purchased, purchase.Tickets...)
		}
	}

	if len(purchased) == 0 {
		log.Fatalf("❌ %d회차 구매 내역을 찾을 수 없습니다 (최근 %d일 조회)", winning.Round, purchaseHistoryDays)
	}

	// 6. Check each ticket and build summary
	summary := domain.NewCheckSummary(winning)
	for _, ticket := range purchased {
		rank := domain.CheckWinning(ticket.Numbers, winning)
		var prize int64
		if rank != domain.RankNone {
			if prizeInfo, ok := winning.Prizes[rank]; ok {
				prize = prizeInfo.AmountPerWinner
			}
		}
		result := domain.NewTicketResult(ticket.Slot, ticket.Mode, ticket.Numbers, rank, prize)
		summary.AddTicket(result)
	}

	log.Printf("\n🎰 [%d]회 당첨 번호 (%s 추첨)", winning.Round, winning.DrawDate.Format("2006-01-02"))
	log.Printf("   당첨 번호: %v", winning.Numbers)
	log.Printf("   보너스: %d", winning.BonusNumber)

	log.Printf("\n💰 [%d회] 당첨금 정보:", winning.Round)
	for rank := domain.Rank1; rank >= domain.Rank5; rank-- {
		if prizeInfo, ok := winning.Prizes[rank]; ok {
			log.Printf("%s", prizeInfo.ToString())
		}
	}

	log.Printf("%s", summary.ToString())
	if summary.HasWinner() {
		log.Println("\n🎉 축하합니다! 당첨되었습니다!")
	} else {
		log.Println("\n😢 당첨되지 않았습니다.")
	}

	if err := emailSender.SendLotteryCheckResultMail(summary); err != nil {
		log.Fatalf("❌ 이메일 전송 실패: %v", err)
	}
	log.Println("✉️  결과 이메일 전송 완료")
}
