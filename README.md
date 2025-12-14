# Weekly Lotto

동행복권 로또6/45 자동 구매 및 당첨 확인 시스템

## 기능

- **자동 구매**: 매주 월~금 오전 9시에 로또 1장씩 구매
- **당첨 확인**: 매주 토요일 추첨 후 당첨 결과 이메일 발송
- **자동 커밋**: 매주 일요일 자동 커밋으로 GitHub Actions 비활성화 방지

## 실행 스케줄

| 작업    | 실행 시간         | 설명                            |
|-------|---------------|-------------------------------|
| 로또 구매 | 월~금 09:00 KST | 로또 1장 자동 구매                   |
| 당첨 확인 | 토요일 21:00 KST | 당첨 결과 확인 및 이메일 발송             |
| 자동 커밋 | 일요일 09:00 KST | README 업데이트 (Actions 비활성화 방지) |

## 환경변수 설정

Repository Settings → Secrets and variables → Actions에서 설정:

### 로또 계정

- `LOTTO_USERNAME`: 동행복권 로그인 아이디
- `LOTTO_PASSWORD`: 동행복권 로그인 비밀번호

### 이메일 알림

- `LOTTO_EMAIL_SMTP_HOST`: SMTP 서버 주소 (예: smtp.gmail.com)
- `LOTTO_EMAIL_SMTP_PORT`: SMTP 포트 (예: 587)
- `LOTTO_EMAIL_USERNAME`: SMTP 인증 계정
- `LOTTO_EMAIL_PASSWORD`: SMTP 인증 비밀번호
- `LOTTO_EMAIL_FROM`: 발신자 이메일
- `LOTTO_EMAIL_TO`: 수신자 이메일
