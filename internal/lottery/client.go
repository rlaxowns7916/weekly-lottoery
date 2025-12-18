package lottery

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"time"

	"weekly-lotto/internal/domain"
	"weekly-lotto/internal/parser"
)

const (
	defaultSessionURL = "https://dhlottery.co.kr/gameResult.do?method=byWin&wiselog=H_C_1_1"
	systemCheckURL    = "https://dhlottery.co.kr/index_check.html"
	mainURL           = "https://www.dhlottery.co.kr/common.do?method=main"
	loginURL          = "https://www.dhlottery.co.kr/userSsl.do?method=login"
	balanceURL        = "https://dhlottery.co.kr/userSsl.do?method=myPage"
	readySocketURL    = "https://ol.dhlottery.co.kr/olotto/game/egovUserReadySocket.json"
	buyLotto645URL    = "https://ol.dhlottery.co.kr/olotto/game/execBuy.do"
	winningURL        = "https://dhlottery.co.kr/gameResult.do?method=byWin"
	lottoBuyListURL   = "https://www.dhlottery.co.kr/myPage.do?method=lottoBuyList"
	lottoDetailURL    = "https://www.dhlottery.co.kr/myPage.do?method=lotto645Detail"
)

// Client handles HTTP communication with the lottery website.
type Client struct {
	httpClient *http.Client
	username   string
	password   string
}

// NewClient creates a new lottery client and initializes session.
// It automatically performs session initialization and login.
func NewClient(username, password string) (*Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("쿠키 jar 생성 실패: %w", err)
	}

	client := &Client{
		httpClient: &http.Client{
			Jar: jar,
		},
		username: username,
		password: password,
	}

	// 세션 초기화
	if err := client.initSession(); err != nil {
		return nil, fmt.Errorf("세션 초기화 실패: %w", err)
	}

	// 로그인
	if err := client.login(); err != nil {
		return nil, fmt.Errorf("로그인 실패: %w", err)
	}

	return client, nil
}

// initSession obtains JSESSIONID cookie.
func (c *Client) initSession() error {
	req, err := http.NewRequest("GET", defaultSessionURL, nil)
	if err != nil {
		return err
	}

	c.setDefaultHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// 시스템 점검 페이지로 리다이렉트되었는지 확인
	if resp.Request.URL.String() == systemCheckURL {
		return fmt.Errorf("동행복권 사이트가 현재 시스템 점검중입니다")
	}

	// JSESSIONID 쿠키는 자동으로 jar에 저장됨
	return nil
}

// login performs user authentication.
func (c *Client) login() error {
	formData := url.Values{}
	formData.Set("returnUrl", mainURL)
	formData.Set("userId", c.username)
	formData.Set("password", c.password)
	formData.Set("checkSave", "off")
	formData.Set("newsEventYn", "")

	req, err := http.NewRequest("POST", loginURL, bytes.NewBufferString(formData.Encode()))
	if err != nil {
		return err
	}

	c.setDefaultHeaders(req)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// 로그인 결과 파싱 (실패 시 에러 반환)
	return parser.ParseLoginResult(resp.Body)
}

// GetCurrentRound retrieves the next lottery round number.
func (c *Client) GetCurrentRound() (int, error) {
	req, err := http.NewRequest("GET", mainURL, nil)
	if err != nil {
		return 0, err
	}

	c.setDefaultHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	return parser.ParseCurrentRound(resp.Body)
}

// setDefaultHeaders sets common HTTP headers for requests.
func (c *Client) setDefaultHeaders(req *http.Request) {
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.77 Safari/537.36")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Cache-Control", "max-age=0")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8")
	req.Header.Set("Accept-Language", "ko,en-US;q=0.9,en;q=0.8,ko-KR;q=0.7")
	req.Header.Set("Referer", "https://dhlottery.co.kr")
}

// PurchasedTicket represents a purchased lottery ticket with selected numbers.
type PurchasedTicket struct {
	Round   int
	Slot    string // A, B, C, D, E
	Numbers []int  // 6 numbers
	Mode    string // 자동, 반자동, 수동
}

// PurchaseHistory aggregates tickets for a single purchase order.
type PurchaseHistory struct {
	Round   int
	OrderNo string
	Tickets []PurchasedTicket
}

// BuyLotto645 purchases lottery tickets and returns the purchased numbers.
func (c *Client) BuyLotto645(tickets []*domain.Lotto645Ticket) ([]PurchasedTicket, error) {
	// 1. Get ready_ip
	readyIP, err := c.getReadySocket()
	if err != nil {
		return nil, fmt.Errorf("ready_ip 획득 실패: %w", err)
	}

	// 2. Get current round number
	round, err := c.GetCurrentRound()
	if err != nil {
		return nil, fmt.Errorf("회차 정보 조회 실패: %w", err)
	}

	// 3. Build purchase parameters
	param, err := c.makeBuyParam(tickets)
	if err != nil {
		return nil, fmt.Errorf("구매 파라미터 생성 실패: %w", err)
	}

	// 4. Build form data
	formData := url.Values{}
	formData.Set("round", strconv.Itoa(round))
	formData.Set("direct", readyIP)
	formData.Set("nBuyAmount", strconv.Itoa(1000*len(tickets)))
	formData.Set("param", param)
	formData.Set("gameCnt", strconv.Itoa(len(tickets)))

	// 5. Send purchase request
	req, err := http.NewRequest("POST", buyLotto645URL, bytes.NewBufferString(formData.Encode()))
	if err != nil {
		return nil, err
	}

	c.setDefaultHeaders(req)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// 6. Parse response
	var result struct {
		Result struct {
			ResultCode       string   `json:"resultCode"`
			ResultMsg        string   `json:"resultMsg"`
			ArrGameChoiceNum []string `json:"arrGameChoiceNum"`
		} `json:"result"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("응답 파싱 실패: %w", err)
	}

	// 7. Check success
	if result.Result.ResultCode != "100" {
		return nil, fmt.Errorf("구매 실패: %s", result.Result.ResultMsg)
	}

	// 8. Parse purchased numbers
	// Format: ["A|01|02|04|27|39|443", "B|11|23|25|27|28|452"]
	purchased := parsePurchasedNumbers(round, result.Result.ArrGameChoiceNum)

	return purchased, nil
}

// getReadySocket retrieves the ready_ip for purchase.
func (c *Client) getReadySocket() (string, error) {
	req, err := http.NewRequest("POST", readySocketURL, nil)
	if err != nil {
		return "", err
	}

	c.setDefaultHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		ReadyIP string `json:"ready_ip"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("ready_ip 응답 파싱 실패: %w", err)
	}

	return result.ReadyIP, nil
}

// makeBuyParam converts tickets to JSON parameter string.
func (c *Client) makeBuyParam(tickets []*domain.Lotto645Ticket) (string, error) {
	slots := make([]map[string]interface{}, len(tickets))
	slotNames := []string{"A", "B", "C", "D", "E"}

	for i, ticket := range tickets {
		if i >= len(slotNames) {
			return "", fmt.Errorf("최대 5장까지만 구매 가능합니다")
		}

		var genType string
		var arrGameChoiceNum interface{}

		switch ticket.Mode {
		case domain.ModeAuto:
			genType = "0"
			arrGameChoiceNum = nil
		case domain.ModeManual:
			genType = "1"
			arrGameChoiceNum = numbersToString(ticket.Numbers)
		case domain.ModeSemiAuto:
			genType = "2"
			arrGameChoiceNum = numbersToString(ticket.Numbers)
		default:
			return "", fmt.Errorf("올바르지 않은 모드입니다: %v", ticket.Mode)
		}

		slots[i] = map[string]interface{}{
			"genType":          genType,
			"arrGameChoiceNum": arrGameChoiceNum,
			"alpabet":          slotNames[i],
		}
	}

	data, err := json.Marshal(slots)
	if err != nil {
		return "", fmt.Errorf("JSON 직렬화 실패: %w", err)
	}

	return string(data), nil
}

// numbersToString converts []int to comma-separated string.
// Example: [1, 2, 3] -> "1,2,3"
func numbersToString(numbers []int) string {
	strs := make([]string, len(numbers))
	for i, n := range numbers {
		strs[i] = strconv.Itoa(n)
	}
	return strings.Join(strs, ",")
}

// GetWinningNumbers retrieves the latest winning numbers.
func (c *Client) GetWinningNumbers() (*domain.WinningNumbers, error) {
	req, err := http.NewRequest("GET", winningURL, nil)
	if err != nil {
		return nil, err
	}

	c.setDefaultHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return parser.ParseWinningNumbers(resp.Body)
}

// GetRecentPurchases retrieves purchase history within the given number of days.
func (c *Client) GetRecentPurchases(days int) ([]PurchaseHistory, error) {
	end := time.Now()
	start := end.AddDate(0, 0, -days)

	summaries, err := c.fetchPurchaseSummaries(start, end)
	if err != nil {
		return nil, fmt.Errorf("구매 내역 조회 실패: %w", err)
	}

	histories := make([]PurchaseHistory, 0, len(summaries))
	for _, summary := range summaries {
		round, tickets, err := c.fetchPurchaseTickets(summary)
		if err != nil {
			return nil, fmt.Errorf("구매 상세 조회 실패 (orderNo: %v, err :%v)", summary.OrderNo, err)
		}

		if round == 0 {
			return nil, fmt.Errorf("구매 상세 조회 - 회차 조회 실패 (orderNo: %v)")
		}

		histories = append(histories, PurchaseHistory{
			Round:   round,
			OrderNo: summary.OrderNo,
			Tickets: tickets,
		})
	}

	if len(histories) == 0 {
		return nil, fmt.Errorf("구매 내역을 찾을 수 없습니다")
	}

	return histories, nil
}

func (c *Client) fetchPurchaseSummaries(start, end time.Time) ([]parser.PurchaseSummary, error) {
	formData := url.Values{}
	formData.Set("nowPage", "1")
	formData.Set("searchStartDate", start.Format("20060102"))
	formData.Set("searchEndDate", end.Format("20060102"))
	formData.Set("lottoId", "")
	formData.Set("winGrade", "2")
	formData.Set("calendarStartDt", start.Format("2006-01-02"))
	formData.Set("calendarEndDt", end.Format("2006-01-02"))
	formData.Set("sortOrder", "DESC")

	req, err := http.NewRequest("POST", lottoBuyListURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return nil, err
	}

	c.setDefaultHeaders(req)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return parser.ParsePurchaseList(resp.Body)
}

func (c *Client) fetchPurchaseTickets(summary parser.PurchaseSummary) (int, []PurchasedTicket, error) {
	parsedURL, err := url.Parse(lottoDetailURL)
	if err != nil {
		return 0, nil, err
	}

	q := parsedURL.Query()
	q.Set("orderNo", summary.OrderNo)
	q.Set("barcode", summary.Barcode)
	q.Set("issueNo", summary.IssueNo)
	parsedURL.RawQuery = q.Encode()

	req, err := http.NewRequest("GET", parsedURL.String(), nil)
	if err != nil {
		return 0, nil, err
	}

	c.setDefaultHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	round, details, err := parser.ParsePurchaseDetail(resp.Body)
	if err != nil {
		return 0, nil, err
	}

	tickets := make([]PurchasedTicket, 0, len(details))
	for _, detail := range details {
		tickets = append(tickets, PurchasedTicket{
			Round:   round,
			Slot:    detail.Slot,
			Numbers: detail.Numbers,
			Mode:    detail.Mode,
		})
	}

	return round, tickets, nil
}

// parsePurchasedNumbers parses purchased number strings.
// Format: ["A|01|02|04|27|39|443", "B|11|23|25|27|28|452"]
// slot[0] = A, slot[2:-1] = numbers, slot[-1] = mode (1=수동, 2=반자동, 3=자동)
func parsePurchasedNumbers(
	round int,
	lines []string,
) []PurchasedTicket {
	modeMap := map[string]string{
		"1": "수동",
		"2": "반자동",
		"3": "자동",
	}

	tickets := make([]PurchasedTicket, 0, len(lines))
	for _, line := range lines {
		if len(line) < 3 {
			continue
		}

		// Parse: "A|01|02|04|27|39|443"
		slot := line[:1]                        // first rune (A~E)
		modeCode := line[len(line)-1:]          // last character (1,2,3)
		numbersSection := line[2 : len(line)-1] // strip "A|" prefix and mode suffix
		numberParts := strings.Split(numbersSection, "|")

		numbers := make([]int, 0, len(numberParts))
		for _, numStr := range numberParts {
			if num, err := strconv.Atoi(numStr); err == nil {
				numbers = append(numbers, num)
			}
		}

		mode := modeMap[modeCode]
		if mode == "" {
			mode = "알 수 없음"
		}

		tickets = append(tickets, PurchasedTicket{
			Round:   round,
			Slot:    slot,
			Numbers: numbers,
			Mode:    mode,
		})
	}

	return tickets
}
