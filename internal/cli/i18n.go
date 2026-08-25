package cli

type cliText struct {
	rootShort     string
	rootLong      string
	helpFlag      string
	usageTemplate string

	helpCommandShort string
	helpCommandLong  string
	helpUnknownTopic string

	completionShort      string
	completionLong       string
	completionNoDescFlag string
	completionShellShort string
	completionShellLong  string

	versionShort string

	statusShort       string
	statusLong        string
	statusVerboseFlag string
	statusJSONFlag    string
	statusFetchingFmt string

	// Text-mode usage rendering (status).
	statusErrorFmt            string // provider name, error
	statusSubAccessError      string // translation of provider.ClaudeSubscriptionAccessError; empty = print the error's own text
	statusFiveHourLineFmt     string // formatted window
	statusWeeklyLineFmt       string // formatted window
	statusNotEnforced         string
	statusWindowFmt           string // bar, pct, display word, countdown, clock
	statusWindowNoResetFmt    string // bar, pct, display word
	statusUsedWord            string
	statusRemainingWord       string
	statusCreditsUnlimited    string
	statusCreditsFmt          string // balance
	statusResetCreditsOneFmt  string // count (1)
	statusResetCreditsManyFmt string // count (>1)
	statusCreditAvailable     string
	statusCreditRedeemed      string
	statusCreditExpired       string
	statusCreditGrantedFmt    string // datetime
	statusCreditExpiresFmt    string // datetime
	statusCreditExpiresInFmt  string // remaining duration, appended to the expires part
	statusCreditRedeemedFmt   string // datetime
	statusCreditTimeLayout    string
	statusListSep             string
	statusNowWord             string
	statusWeekdays            [7]string // Sunday first

	pingShort       string
	pingLong        string
	pingDryRunFlag  string
	pingWouldRunFmt string // provider, command
	pingSendingFmt  string // provider, spinner frame, elapsed
	pingFailedFmt   string // provider, elapsed, error
	pingSuccessFmt  string // provider, elapsed, usage suffix

	watchShort             string
	watchLong              string
	watchDryRunFlag        string
	watchLiveFlag          string
	watchAlreadyRunningFmt string

	// `redeem` reset-credit strings.
	redeemShort         string
	redeemLong          string
	redeemDryRunFlag    string
	redeemNoneAvailable string
	redeemPlanFmt       string // expiry stamp, remaining lifetime
	redeemDryRunNote    string
	redeemOutcomeFmt    string // outcome sentence
	redeemDone          string
	redeemNothing       string
	redeemNoCredit      string
	redeemAlready       string
	redeemUnknownFmt    string // raw outcome code

	configShort     string
	configInitShort string
	configInitForce string
	configPathShort string
}

// localizedText returns the CLI's (Korean-only) text. There is no longer any
// locale detection — claude-window-keeper is a single-operator tool.
func localizedText() cliText {
	return koreanText
}

var koreanText = cliText{
	rootShort: "Claude Code / Codex / Spark 요청 제한 윈도우를 끊김 없이 이어줍니다",
	rootLong:  "limitping은 AI 코딩 프로바이더의 5시간 요청 제한 윈도우가 리셋되는 순간 바로 ping을 보내, 다음 윈도우가 즉시 시작되고 정렬 상태를 유지하도록 합니다. 사용량은 쿼터를 소모하지 않는 엔드포인트로 조회하며, ping은 공식 CLI를 통해 전송됩니다.",
	helpFlag:  "이 명령어의 도움말 표시",
	usageTemplate: `사용법:{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}}{{if gt (len .Aliases) 0}}

별칭:
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

예시:
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}{{$cmds := .Commands}}{{if eq (len .Groups) 0}}

사용 가능한 명령어:{{range $cmds}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .NameAndAliases 24}} {{.Short}}{{end}}{{end}}{{else}}{{range $group := .Groups}}

{{.Title}}{{range $cmds}}{{if (and (eq .GroupID $group.ID) (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .NameAndAliases 24}} {{.Short}}{{end}}{{end}}{{end}}{{if not .AllChildCommandsHaveGroup}}

기타 명령어:{{range $cmds}}{{if (and (eq .GroupID "") (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .NameAndAliases 24}} {{.Short}}{{end}}{{end}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

플래그:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

전역 플래그:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}

기타 도움말 주제:{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

"{{.CommandPath}} [command] --help" 명령으로 해당 명령어의 자세한 정보를 확인하세요.{{end}}
`,

	helpCommandShort: "임의의 명령어에 대한 도움말",
	helpCommandLong:  "애플리케이션의 모든 명령어에 대한 도움말을 제공합니다.\nlimitping help [command] 형태로 입력하면 전체 상세 정보를 볼 수 있습니다.",
	helpUnknownTopic: "알 수 없는 도움말 주제",

	completionShort:      "셸 자동완성 스크립트 생성",
	completionLong:       "limitping의 셸 자동완성 스크립트를 생성합니다.\n\n셸별 사용법은 `limitping completion [bash|zsh|fish|powershell] --help`를 실행해 확인하세요.",
	completionNoDescFlag: "자동완성 설명 비활성화",
	completionShellShort: "%s 셸 자동완성 스크립트 생성",
	completionShellLong:  "limitping의 %s 셸 자동완성 스크립트를 생성합니다.",

	versionShort: "버전 출력",

	statusShort:       "쿼터를 소모하지 않고 현재 5시간/주간 사용량과 리셋 카운트다운 확인",
	statusLong:        "활성화된 모든 프로바이더의 현재 5시간 및 주간 사용량을 확인합니다. 이 명령어는 쿼터를 소모하지 않는 엔드포인트에서 사용량 데이터만 읽으며, ping을 보내거나 모델 쿼터를 소모하지 않습니다.",
	statusVerboseFlag: "원본 JSON 응답 출력",
	statusJSONFlag:    "텍스트 대신 JSON 형식으로 사용량 출력",
	statusFetchingFmt: "%s 사용량 조회 중...\n",

	statusErrorFmt:            "%-7s  오류: %v\n",
	statusSubAccessError:      "Claude 구독 접근이 불가능합니다 (구독이 만료/갱신 실패했거나, 조직 관리자가 Claude Code를 비활성화했을 수 있습니다). 구독을 복구하거나 Claude Code에서 Anthropic API Key를 대신 사용하세요",
	statusFiveHourLineFmt:     "  5h     %s\n",
	statusWeeklyLineFmt:       "  주     %s\n",
	statusNotEnforced:         "현재 적용되지 않음",
	statusWindowFmt:           "%s %5.1f%% %s  %s 후 리셋 (%s)",
	statusWindowNoResetFmt:    "%s %5.1f%% %s  (활성 윈도우 없음)",
	statusUsedWord:            "사용",
	statusRemainingWord:       "잔여",
	statusCreditsUnlimited:    "  크레딧 무제한\n",
	statusCreditsFmt:          "  크레딧 %s\n",
	statusResetCreditsOneFmt:  "  리셋 크레딧 %d개 사용 가능\n",
	statusResetCreditsManyFmt: "  리셋 크레딧 %d개 사용 가능\n",
	statusCreditAvailable:     "사용 가능",
	statusCreditRedeemed:      "사용됨",
	statusCreditExpired:       "만료됨",
	statusCreditGrantedFmt:    "발급일 %s",
	statusCreditExpiresFmt:    "만료일 %s",
	statusCreditExpiresInFmt:  " (남은 기간 %s)",
	statusCreditRedeemedFmt:   "사용일 %s",
	statusCreditTimeLayout:    "01-02 15:04",
	statusListSep:             ", ",
	statusNowWord:             "지금",
	statusWeekdays:            [7]string{"일", "월", "화", "수", "목", "금", "토"},

	pingShort: "최소한의 메시지로 프로바이더 윈도우 즉시 트리거",
	pingLong: `지정한 프로바이더에 최소한의 메시지를 보내 요청 제한 윈도우를 즉시 트리거합니다.

인자:
  provider  선택. claude, codex, spark, all 중 하나.
            기본값은 all이며, 활성화된 모든 프로바이더에 ping을 보냅니다.

예시:
  limitping ping
  limitping p claude
  limitping ping codex --dry-run`,
	pingDryRunFlag:  "전송하지 않고 실행될 명령어만 출력",
	pingWouldRunFmt: "%-7s 실행 예정: %s\n",
	pingSendingFmt:  "\r%-7s %c 전송 중… %s",
	pingFailedFmt:   "%-7s ✗ 실패 (소요 %s): %v\n",
	pingSuccessFmt:  "%-7s ✓ ping 완료 (%s%s)\n",

	watchShort: "포그라운드 데몬을 실행하며 각 프로바이더의 5시간 윈도우가 리셋될 때 ping 전송",
	watchLong: `포그라운드 데몬을 실행합니다. 프로바이더의 5시간 윈도우가 리셋되면 limitping이 최소한의 메시지를 보내 다음 윈도우를 시작합니다.

인자:
  provider  선택. claude, codex, spark, all 중 하나.
            기본값은 all이며, 활성화된 모든 프로바이더를 감시합니다.

예시:
  limitping watch
  limitping w claude
  limitping watch --live
  limitping watch --dry-run`,
	watchDryRunFlag:        "전송하지 않고 트리거될 시점만 로그로 기록",
	watchLiveFlag:          "감시 중 실시간 하트비트/상태 라인 표시 (전력 소모 증가)",
	watchAlreadyRunningFmt: "watch가 이미 실행 중입니다 (pid %d, provider %s%s, 시작 시각 %s). 새로 시작하기 전에 기존 watcher를 먼저 중지하세요",

	redeemShort: "적립된 Codex 요청 제한 리셋 크레딧을 지금 사용",
	redeemLong: `'limitping status'에 표시된 Codex 리셋 크레딧 하나를 사용하여, 해당 크레딧이 적용 가능한 요청 제한 윈도우를 리셋합니다.

크레딧 사용은 되돌릴 수 없습니다. 어떤 크레딧을 사용할지는 백엔드가 결정하며, 현재 리셋 가능한 윈도우가 없으면 "nothing to reset"으로 거부되므로 크레딧이 헛되이 소모되지 않습니다.

설정 파일의 [codex] 아래에 auto_redeem = true를 설정하면, 크레딧이 만료에 가까워졌을 때(만료까지 24시간 이내이면서 실제로 회수할 사용량이 있거나, 마지막 1시간 이내) 'watch'가 자동으로 크레딧을 사용하게 할 수 있습니다.

예시:
  limitping redeem --dry-run
  limitping redeem`,
	redeemDryRunFlag:    "실제로 사용하지 않고 어떤 크레딧이 사용될지만 표시",
	redeemNoneAvailable: "사용 가능한 리셋 크레딧이 없습니다",
	redeemPlanFmt:       "codex   리셋 크레딧 1개를 사용합니다 (만료일 %s, 남은 기간 %s)\n",
	redeemDryRunNote:    "dry run: 아무것도 소모되지 않았습니다\n",
	redeemOutcomeFmt:    "codex   %s\n",
	redeemDone:          "사용 완료 — 적용 가능한 요청 제한 윈도우가 리셋되었습니다",
	redeemNothing:       "현재 리셋 가능한 요청 제한 윈도우가 없어 크레딧이 사용되지 않았습니다",
	redeemNoCredit:      "계정에 사용 가능한 리셋 크레딧이 없습니다",
	redeemAlready:       "이 요청은 이전에 이미 처리되었습니다",
	redeemUnknownFmt:    "백엔드에서 알 수 없는 결과를 반환했습니다: %s",

	configShort:     "설정 파일 관리",
	configInitShort: "기본 설정 파일 작성",
	configInitForce: "기존 설정 파일 덮어쓰기",
	configPathShort: "설정 파일 경로 출력",
}
