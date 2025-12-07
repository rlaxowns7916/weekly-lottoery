package notify

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"html/template"
	"net/smtp"
	"strings"

	"weekly-lotto/internal/config"
	"weekly-lotto/internal/domain"
	domainutils "weekly-lotto/internal/domain/utils"
	"weekly-lotto/internal/lottery"
)

// EmailSender sends notifications via SMTP.
type EmailSender struct {
	cfg *config.EmailConfig
}

// NewEmailSender creates a sender using the provided configuration.
func NewEmailSender(cfg *config.EmailConfig) *EmailSender {
	return &EmailSender{cfg: cfg}
}

// SendLotteryBuyMail notifies purchased ticket numbers.
func (s *EmailSender) SendLotteryBuyMail(tickets []lottery.PurchasedTicket) error {
	if len(tickets) == 0 {
		return fmt.Errorf("구매한 티켓이 없습니다")
	}

	body, err := renderBuyEmail(tickets)
	if err != nil {
		return err
	}

	round := tickets[0].Round
	subject := fmt.Sprintf("[weekly-lotto] %d회 로또 %d장 구매 완료", round, len(tickets))
	return s.send(subject, body, "text/html; charset=UTF-8")
}

// SendLotteryCheckResultMail notifies winning check results.
func (s *EmailSender) SendLotteryCheckResultMail(summary *domain.CheckSummary) error {
	if summary == nil {
		return fmt.Errorf("check summary가 비어 있습니다")
	}

	body, err := renderCheckResultEmail(summary)
	if err != nil {
		return err
	}

	subject := fmt.Sprintf("[weekly-lotto] %d회 당첨 결과", summary.Round)
	return s.send(subject, body, "text/html; charset=UTF-8")
}

// SendFailureNotification sends error notification email.
func (s *EmailSender) SendFailureNotification(operation string, errorMsg string) error {
	body, err := renderFailureEmail(operation, errorMsg)
	if err != nil {
		return err
	}

	subject := fmt.Sprintf("[weekly-lotto] ❌ %s 실패", operation)
	return s.send(subject, body, "text/html; charset=UTF-8")
}

// send dispatches an email with the given subject and body.
func (s *EmailSender) send(subject, body, contentType string) error {
	if contentType == "" {
		contentType = "text/plain; charset=UTF-8"
	}
	headers := []string{
		fmt.Sprintf("From: %s", s.cfg.From),
		fmt.Sprintf("To: %s", strings.Join(s.cfg.To, ", ")),
		fmt.Sprintf("Subject: %s", subject),
		"MIME-Version: 1.0",
		fmt.Sprintf("Content-Type: %s", contentType),
	}

	message := strings.Join(headers, "\r\n") + "\r\n\r\n" + body
	addr := fmt.Sprintf("%s:%d", s.cfg.SMTPHost, s.cfg.SMTPPort)

	// AIDEV-NOTE: 포트 465 (implicit TLS) 지원
	// 포트 465는 연결 시작부터 TLS가 필요하므로 직접 TLS 다이얼 후 SMTP 통신
	// 포트 587 (STARTTLS)은 smtp.SendMail이 자동 처리
	if s.cfg.SMTPPort == 465 {
		tlsConfig := &tls.Config{
			ServerName:         s.cfg.SMTPHost,
			InsecureSkipVerify: false, // 프로덕션: 인증서 검증 필수
			MinVersion:         tls.VersionTLS12,
		}
		conn, err := tls.Dial("tcp", addr, tlsConfig)
		if err != nil {
			return fmt.Errorf("TLS 연결 실패: %w", err)
		}
		defer conn.Close()

		client, err := smtp.NewClient(conn, s.cfg.SMTPHost)
		if err != nil {
			return fmt.Errorf("SMTP 클라이언트 생성 실패: %w", err)
		}
		defer client.Close()

		auth := smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.SMTPHost)
		if err = client.Auth(auth); err != nil {
			return fmt.Errorf("인증 실패: %w", err)
		}

		if err = client.Mail(s.cfg.From); err != nil {
			return fmt.Errorf("MAIL FROM 실패: %w", err)
		}
		for _, to := range s.cfg.To {
			if err = client.Rcpt(to); err != nil {
				return fmt.Errorf("RCPT TO 실패 (%s): %w", to, err)
			}
		}

		w, err := client.Data()
		if err != nil {
			return fmt.Errorf("DATA 명령 실패: %w", err)
		}
		_, err = w.Write([]byte(message))
		if err != nil {
			return fmt.Errorf("메시지 쓰기 실패: %w", err)
		}
		err = w.Close()
		if err != nil {
			return fmt.Errorf("메시지 종료 실패: %w", err)
		}

		return client.Quit()
	}

	// 포트 587 (STARTTLS) 또는 포트 25는 기존 방식 사용
	auth := smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.SMTPHost)
	return smtp.SendMail(addr, auth, s.cfg.From, s.cfg.To, []byte(message))
}

func renderCheckResultEmail(summary *domain.CheckSummary) (string, error) {
	data := checkResultTemplateData{
		Round:       summary.Round,
		DrawDate:    summary.DrawDate.Format("2006-01-02"),
		Numbers:     append([]int(nil), summary.WinningNumbers...),
		BonusNumber: summary.BonusNumber,
		HasWinner:   summary.HasWinner(),
		SummaryText: strings.TrimSpace(summary.ToString()),
	}

	if len(summary.Prizes) > 0 {
		data.Prizes = make([]checkResultTemplatePrize, 0, len(summary.Prizes))
		for rank := domain.Rank1; rank >= domain.Rank5; rank-- {
			if prize, ok := summary.Prizes[rank]; ok {
				data.Prizes = append(data.Prizes, checkResultTemplatePrize{
					RankLabel:   prize.Rank.String(),
					WinnerCount: prize.WinnerCount,
					PrizeAmount: fmt.Sprintf("%s원", domainutils.FormatAmount(prize.AmountPerWinner)),
					TotalAmount: fmt.Sprintf("%s원", domainutils.FormatAmount(prize.TotalAmount)),
				})
			}
		}
	}

	var buf bytes.Buffer
	if err := checkResultTemplate.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("당첨 결과 템플릿 렌더링 실패: %w", err)
	}

	return buf.String(), nil
}

type checkResultTemplatePrize struct {
	RankLabel   string
	WinnerCount int
	PrizeAmount string
	TotalAmount string
}

type checkResultTemplateData struct {
	Round       int
	DrawDate    string
	Numbers     []int
	BonusNumber int
	HasWinner   bool
	Prizes      []checkResultTemplatePrize
	SummaryText string
}

var checkResultTemplate = template.Must(template.New("lotto-check-result").Parse(checkResultTemplateHTML))

const checkResultTemplateHTML = `<!DOCTYPE html>
<html lang="ko">
<head>
  <meta charset="UTF-8" />
  <title>로또 {{.Round}}회 당첨 결과 안내</title>
  <style>
    /* 기본 레이아웃 */
    body {
      margin: 0;
      padding: 0;
      background-color: #f4f4f5;
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Noto Sans KR",
        "Apple SD Gothic Neo", sans-serif;
    }
    .wrapper {
      width: 100%;
      padding: 24px 0;
    }
    .container {
      max-width: 600px;
      margin: 0 auto;
      background-color: #ffffff;
      border-radius: 12px;
      padding: 24px 24px 32px;
      box-shadow: 0 4px 16px rgba(15, 23, 42, 0.08);
    }

    /* 헤더 */
    .header {
      text-align: center;
      margin-bottom: 24px;
    }
    .badge {
      display: inline-block;
      padding: 4px 12px;
      border-radius: 999px;
      background: #eef2ff;
      color: #4f46e5;
      font-size: 12px;
      font-weight: 600;
      letter-spacing: 0.03em;
    }
    h1 {
      font-size: 22px;
      margin: 12px 0 4px;
      color: #111827;
    }
    .sub {
      font-size: 13px;
      color: #6b7280;
    }

    /* 번호 영역 */
    .numbers {
      margin: 24px 0 16px;
      text-align: center;
    }
    .numbers-label {
      font-size: 13px;
      color: #6b7280;
      margin-bottom: 8px;
    }
    .ball {
      display: inline-block;
      width: 36px;
      height: 36px;
      line-height: 36px;
      margin: 0 4px;
      border-radius: 999px;
      background: #f97316;
      color: #ffffff;
      font-weight: 700;
      font-size: 16px;
    }
    .ball.bonus {
      background: #4b5563;
      margin-left: 10px;
    }

    /* 당첨 여부 */
    .status-success {
      padding: 10px 12px;
      border-radius: 10px;
      background: #ecfdf3;
      color: #166534;
      font-size: 14px;
      font-weight: 600;
      margin-bottom: 12px;
    }
    .status-fail {
      padding: 10px 12px;
      border-radius: 10px;
      background: #fef2f2;
      color: #b91c1c;
      font-size: 14px;
      font-weight: 600;
      margin-bottom: 12px;
    }

    /* 당첨금 테이블 */
    .section-title {
      font-size: 14px;
      font-weight: 600;
      color: #111827;
      margin: 20px 0 8px;
    }
    .prize-table {
      width: 100%;
      border-collapse: collapse;
      margin: 4px 0 20px;
      font-size: 13px;
    }
    .prize-table th,
    .prize-table td {
      padding: 8px 10px;
      border-bottom: 1px solid #e5e7eb;
      text-align: right;
      white-space: nowrap;
    }
    .prize-table th:first-child,
    .prize-table td:first-child {
      text-align: left;
    }
    .prize-table thead {
      background: #f9fafb;
    }

    /* 요약 */
    .summary-box {
      padding: 12px 12px 10px;
      border-radius: 10px;
      background: #f9fafb;
      font-size: 13px;
      color: #374151;
      line-height: 1.6;
      white-space: pre-line;
    }

    /* 푸터 */
    .footer {
      margin-top: 24px;
      font-size: 11px;
      color: #9ca3af;
      text-align: center;
      line-height: 1.5;
    }

    /* 모바일 대응 */
    @media (max-width: 640px) {
      .container {
        border-radius: 0;
        padding: 18px 16px 24px;
      }
      .ball {
        width: 32px;
        height: 32px;
        line-height: 32px;
        font-size: 14px;
      }
    }
  </style>
</head>
<body>
  <div class="wrapper">
    <div class="container">
      <!-- 헤더 -->
      <div class="header">
        <div class="badge">🎰 로또 자동 추첨 결과</div>
        <h1>{{.Round}}회 당첨 결과 안내</h1>
        <div class="sub">{{.DrawDate}} 추첨 기준</div>
      </div>

      <!-- 당첨 번호 -->
      <div class="numbers">
        <div class="numbers-label">당첨 번호</div>
        {{range .Numbers}}
          <span class="ball">{{.}}</span>
        {{end}}
        <div style="margin-top: 10px; font-size: 12px; color: #6b7280;">
          보너스 번호:
          <span class="ball bonus">{{.BonusNumber}}</span>
        </div>
      </div>

      <!-- 당첨 여부 -->
      {{if .HasWinner}}
        <div class="status-success">
          🎉 축하합니다! 이번 회차에서 당첨 번호가 포함되어 있습니다.
        </div>
      {{else}}
        <div class="status-fail">
          😢 아쉽게도 이번 회차에서는 당첨되지 않았습니다.
        </div>
      {{end}}

      <!-- 당첨금 정보 -->
      {{if .Prizes}}
        <div class="section-title">💰 당첨금 정보</div>
        <table class="prize-table" role="presentation">
          <thead>
            <tr>
              <th>등수</th>
              <th>당첨 인원</th>
              <th>1인당 당첨금</th>
            </tr>
          </thead>
          <tbody>
            {{range .Prizes}}
              <tr>
                <td>{{.RankLabel}}</td>
                <td>{{.WinnerCount}}명</td>
                <td>{{.PrizeAmount}}</td>
              </tr>
            {{end}}
          </tbody>
        </table>
      {{end}}

      <!-- 요약(summary.ToString()) -->
      <div class="section-title">📊 요약</div>
      <div class="summary-box">
        {{.SummaryText}}
      </div>

      <!-- 푸터 -->
      <div class="footer">
        이 메일은 로또 자동 확인 기능에 의해 발송되었습니다.<br />
        본 메일은 발신 전용이며 회신이 되지 않습니다.
      </div>
    </div>
  </div>
</body>
</html>`

func renderBuyEmail(tickets []lottery.PurchasedTicket) (string, error) {
	if len(tickets) == 0 {
		return "", fmt.Errorf("구매한 티켓이 없습니다")
	}

	round := tickets[0].Round
	ticketList := make([]buyTemplateTicket, 0, len(tickets))

	for _, ticket := range tickets {
		ticketList = append(ticketList, buyTemplateTicket{
			Slot:    ticket.Slot,
			Mode:    ticket.Mode,
			Numbers: append([]int(nil), ticket.Numbers...),
		})
	}

	data := buyTemplateData{
		Round:       round,
		TicketCount: len(tickets),
		Tickets:     ticketList,
	}

	var buf bytes.Buffer
	if err := buyTemplate.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("구매 내역 템플릿 렌더링 실패: %w", err)
	}

	return buf.String(), nil
}

type buyTemplateTicket struct {
	Slot    string
	Mode    string
	Numbers []int
}

type buyTemplateData struct {
	Round       int
	TicketCount int
	Tickets     []buyTemplateTicket
}

var buyTemplate = template.Must(template.New("lotto-buy").Parse(buyTemplateHTML))

const buyTemplateHTML = `<!DOCTYPE html>
<html lang="ko">
<head>
  <meta charset="UTF-8" />
  <title>로또 {{.Round}}회 구매 완료</title>
  <style>
    /* 기본 레이아웃 */
    body {
      margin: 0;
      padding: 0;
      background-color: #f4f4f5;
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Noto Sans KR",
        "Apple SD Gothic Neo", sans-serif;
    }
    .wrapper {
      width: 100%;
      padding: 24px 0;
    }
    .container {
      max-width: 600px;
      margin: 0 auto;
      background-color: #ffffff;
      border-radius: 12px;
      padding: 24px 24px 32px;
      box-shadow: 0 4px 16px rgba(15, 23, 42, 0.08);
    }

    /* 헤더 */
    .header {
      text-align: center;
      margin-bottom: 24px;
    }
    .badge {
      display: inline-block;
      padding: 4px 12px;
      border-radius: 999px;
      background: #dcfce7;
      color: #166534;
      font-size: 12px;
      font-weight: 600;
      letter-spacing: 0.03em;
    }
    h1 {
      font-size: 22px;
      margin: 12px 0 4px;
      color: #111827;
    }
    .sub {
      font-size: 13px;
      color: #6b7280;
    }

    /* 티켓 카드 */
    .ticket-list {
      margin: 20px 0;
    }
    .ticket-card {
      background: #f9fafb;
      border-radius: 10px;
      padding: 16px;
      margin-bottom: 12px;
      border-left: 4px solid #22c55e;
    }
    .ticket-header {
      display: flex;
      justify-content: space-between;
      align-items: center;
      margin-bottom: 10px;
    }
    .slot-label {
      font-size: 16px;
      font-weight: 700;
      color: #111827;
    }
    .mode-badge {
      display: inline-block;
      padding: 2px 8px;
      border-radius: 4px;
      background: #e0e7ff;
      color: #4338ca;
      font-size: 11px;
      font-weight: 600;
    }
    .ticket-numbers {
      display: flex;
      gap: 6px;
      flex-wrap: wrap;
    }
    .ball {
      display: inline-block;
      width: 32px;
      height: 32px;
      line-height: 32px;
      text-align: center;
      border-radius: 999px;
      background: #22c55e;
      color: #ffffff;
      font-weight: 700;
      font-size: 14px;
    }

    /* 요약 정보 */
    .summary {
      margin: 24px 0;
      padding: 16px;
      background: #ecfdf3;
      border-radius: 10px;
      text-align: center;
    }
    .summary-text {
      font-size: 15px;
      color: #166534;
      font-weight: 600;
    }

    /* 푸터 */
    .footer {
      margin-top: 24px;
      font-size: 11px;
      color: #9ca3af;
      text-align: center;
      line-height: 1.5;
    }

    /* 모바일 대응 */
    @media (max-width: 640px) {
      .container {
        border-radius: 0;
        padding: 18px 16px 24px;
      }
      .ball {
        width: 28px;
        height: 28px;
        line-height: 28px;
        font-size: 12px;
      }
    }
  </style>
</head>
<body>
  <div class="wrapper">
    <div class="container">
      <!-- 헤더 -->
      <div class="header">
        <div class="badge">🎰 로또 자동 구매 완료</div>
        <h1>{{.Round}}회 구매 완료</h1>
        <div class="sub">총 {{.TicketCount}}장 구매</div>
      </div>

      <!-- 요약 -->
      <div class="summary">
        <div class="summary-text">
          ✅ {{.Round}}회 로또 {{.TicketCount}}장 구매가 완료되었습니다
        </div>
      </div>

      <!-- 티켓 목록 -->
      <div class="ticket-list">
        {{range .Tickets}}
          <div class="ticket-card">
            <div class="ticket-header">
              <span class="slot-label">슬롯 {{.Slot}}</span>
              <span class="mode-badge">{{.Mode}}</span>
            </div>
            <div class="ticket-numbers">
              {{range .Numbers}}
                <span class="ball">{{.}}</span>
              {{end}}
            </div>
          </div>
        {{end}}
      </div>

      <!-- 푸터 -->
      <div class="footer">
        이 메일은 로또 자동 구매 기능에 의해 발송되었습니다.<br />
        본 메일은 발신 전용이며 회신이 되지 않습니다.
      </div>
    </div>
  </div>
</body>
</html>`

func renderFailureEmail(operation string, errorMsg string) (string, error) {
	data := failureTemplateData{
		Operation: operation,
		ErrorMsg:  errorMsg,
		Timestamp: fmt.Sprintf("%s", "실행 시점"),
	}

	var buf bytes.Buffer
	if err := failureTemplate.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("실패 알림 템플릿 렌더링 실패: %w", err)
	}

	return buf.String(), nil
}

type failureTemplateData struct {
	Operation string
	ErrorMsg  string
	Timestamp string
}

var failureTemplate = template.Must(template.New("lotto-failure").Parse(failureTemplateHTML))

const failureTemplateHTML = `<!DOCTYPE html>
<html lang="ko">
<head>
  <meta charset="UTF-8" />
  <title>로또 {{.Operation}} 실패</title>
  <style>
    /* 기본 레이아웃 */
    body {
      margin: 0;
      padding: 0;
      background-color: #f4f4f5;
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Noto Sans KR",
        "Apple SD Gothic Neo", sans-serif;
    }
    .wrapper {
      width: 100%;
      padding: 24px 0;
    }
    .container {
      max-width: 600px;
      margin: 0 auto;
      background-color: #ffffff;
      border-radius: 12px;
      padding: 24px 24px 32px;
      box-shadow: 0 4px 16px rgba(15, 23, 42, 0.08);
    }

    /* 헤더 */
    .header {
      text-align: center;
      margin-bottom: 24px;
    }
    .badge {
      display: inline-block;
      padding: 4px 12px;
      border-radius: 999px;
      background: #fee2e2;
      color: #991b1b;
      font-size: 12px;
      font-weight: 600;
      letter-spacing: 0.03em;
    }
    h1 {
      font-size: 22px;
      margin: 12px 0 4px;
      color: #111827;
    }
    .sub {
      font-size: 13px;
      color: #6b7280;
    }

    /* 에러 박스 */
    .error-box {
      margin: 24px 0;
      padding: 16px;
      background: #fef2f2;
      border-left: 4px solid #dc2626;
      border-radius: 8px;
    }
    .error-title {
      font-size: 14px;
      font-weight: 600;
      color: #991b1b;
      margin-bottom: 8px;
    }
    .error-message {
      font-size: 13px;
      color: #7f1d1d;
      line-height: 1.6;
      white-space: pre-wrap;
      word-break: break-word;
    }

    /* 안내 */
    .notice-box {
      margin: 20px 0;
      padding: 16px;
      background: #fffbeb;
      border-radius: 8px;
      border-left: 4px solid #f59e0b;
    }
    .notice-title {
      font-size: 14px;
      font-weight: 600;
      color: #92400e;
      margin-bottom: 8px;
    }
    .notice-text {
      font-size: 13px;
      color: #78350f;
      line-height: 1.6;
    }

    /* 푸터 */
    .footer {
      margin-top: 24px;
      font-size: 11px;
      color: #9ca3af;
      text-align: center;
      line-height: 1.5;
    }

    /* 모바일 대응 */
    @media (max-width: 640px) {
      .container {
        border-radius: 0;
        padding: 18px 16px 24px;
      }
    }
  </style>
</head>
<body>
  <div class="wrapper">
    <div class="container">
      <!-- 헤더 -->
      <div class="header">
        <div class="badge">❌ 작업 실패</div>
        <h1>{{.Operation}} 실패</h1>
        <div class="sub">자동 실행 중 오류가 발생했습니다</div>
      </div>

      <!-- 에러 정보 -->
      <div class="error-box">
        <div class="error-title">🔍 오류 내용</div>
        <div class="error-message">{{.ErrorMsg}}</div>
      </div>

      <!-- 안내 -->
      <div class="notice-box">
        <div class="notice-title">⚠️ 조치 안내</div>
        <div class="notice-text">
          • GitHub Actions 워크플로우 로그를 확인해주세요<br />
          • 로또 사이트 점검 여부를 확인해주세요<br />
          • 인증 정보(ID/PW)가 유효한지 확인해주세요<br />
          • 문제가 지속되면 수동으로 재실행해주세요
        </div>
      </div>

      <!-- 푸터 -->
      <div class="footer">
        이 메일은 로또 자동화 시스템에 의해 발송되었습니다.<br />
        본 메일은 발신 전용이며 회신이 되지 않습니다.
      </div>
    </div>
  </div>
</body>
</html>`
