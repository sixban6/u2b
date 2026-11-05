package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"unicode/utf16"
)

var (
	shortcutSignMu    sync.Mutex
	shortcutSignCache = make(map[string][]byte)
)

// CanSignShortcuts reports whether macOS `/usr/bin/shortcuts` is available to sign `.shortcut` files.
func CanSignShortcuts() bool {
	if runtime.GOOS != "darwin" {
		return false
	}
	_, err := exec.LookPath("shortcuts")
	return err == nil
}

// BuildSignedIOSShortcut generates and (on macOS) Apple-signs the "u2b 下载" iOS Shortcut.
// Returns (bytes, isSigned, error).
func BuildSignedIOSShortcut(serverBaseURL, token string) ([]byte, bool, error) {
	cleanBase := strings.TrimRight(strings.TrimSpace(serverBaseURL), "/")
	if cleanBase == "" {
		return nil, false, fmt.Errorf("server base URL cannot be empty")
	}

	cacheKeySum := sha256.Sum256([]byte(cleanBase + "|" + token))
	cacheKey := hex.EncodeToString(cacheKeySum[:8])

	shortcutSignMu.Lock()
	if cached, ok := shortcutSignCache[cacheKey]; ok && len(cached) > 0 {
		shortcutSignMu.Unlock()
		return cached, true, nil
	}
	shortcutSignMu.Unlock()

	xmlStr := GenerateIOSShortcutXML(cleanBase, token)

	if !CanSignShortcuts() {
		return []byte(xmlStr), false, nil
	}

	tmpDir, err := os.MkdirTemp("", "u2b_shortcut_*")
	if err != nil {
		return []byte(xmlStr), false, nil
	}
	defer os.RemoveAll(tmpDir)

	unsignedPath := filepath.Join(tmpDir, "u2b-unsigned.shortcut")
	signedPath := filepath.Join(tmpDir, "u2b 下载.shortcut")

	if err := os.WriteFile(unsignedPath, []byte(xmlStr), 0600); err != nil {
		return []byte(xmlStr), false, nil
	}

	// Convert XML plist to binary1 if plutil is available (maximizes compatibility across macOS versions)
	if plutilPath, err := exec.LookPath("plutil"); err == nil {
		_ = exec.Command(plutilPath, "-convert", "binary1", unsignedPath).Run()
	}

	signCmd := exec.Command("shortcuts", "sign", "--mode", "anyone", "--input", unsignedPath, "--output", signedPath)
	if out, err := signCmd.CombinedOutput(); err != nil {
		return []byte(xmlStr), false, fmt.Errorf("shortcuts sign failed: %w (%s)", err, strings.TrimSpace(string(out)))
	}

	signedBytes, err := os.ReadFile(signedPath)
	if err != nil || len(signedBytes) == 0 {
		return []byte(xmlStr), false, fmt.Errorf("failed to read signed shortcut: %w", err)
	}

	shortcutSignMu.Lock()
	shortcutSignCache[cacheKey] = signedBytes
	shortcutSignMu.Unlock()

	return signedBytes, true, nil
}

// ImportShortcutOnMac builds, signs, and opens the shortcut in macOS Shortcuts.app so iCloud syncs it to iPhone.
func ImportShortcutOnMac(serverBaseURL, token string) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("macOS Shortcuts import is only available on macOS")
	}
	signedBytes, isSigned, err := BuildSignedIOSShortcut(serverBaseURL, token)
	if err != nil || !isSigned {
		if err != nil {
			return err
		}
		return fmt.Errorf("shortcut could not be signed on this Mac")
	}

	homeDir, _ := os.UserHomeDir()
	outDir := filepath.Join(homeDir, "u2b")
	_ = os.MkdirAll(outDir, 0755)
	shortcutFile := filepath.Join(outDir, "u2b 下载.shortcut")
	if err := os.WriteFile(shortcutFile, signedBytes, 0644); err != nil {
		return err
	}
	return exec.Command("open", shortcutFile).Start()
}

func xmlEscapeString(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

func utf16CodeUnitLen(s string) int {
	return len(utf16.Encode([]rune(s)))
}

func renderTokenURLParam(prefix, outputUUID, outputName string) string {
	offset := utf16CodeUnitLen(prefix)
	fullStr := xmlEscapeString(prefix) + "\uFFFC"
	return fmt.Sprintf(`<dict>
					<key>Value</key>
					<dict>
						<key>attachmentsByRange</key>
						<dict>
							<key>{%d, 1}</key>
							<dict>
								<key>OutputName</key>
								<string>%s</string>
								<key>OutputUUID</key>
								<string>%s</string>
								<key>Type</key>
								<string>ActionOutput</string>
							</dict>
						</dict>
						<key>string</key>
						<string>%s</string>
					</dict>
					<key>WFSerializationType</key>
					<string>WFTextTokenString</string>
				</dict>`, offset, xmlEscapeString(outputName), xmlEscapeString(outputUUID), fullStr)
}

// GenerateIOSShortcutXML generates a complete, valid Apple Shortcuts XML plist for "u2b 下载".
func GenerateIOSShortcutXML(serverBaseURL, token string) string {
	base := strings.TrimRight(strings.TrimSpace(serverBaseURL), "/")
	tokParam := ""
	panelTokParam := ""
	if strings.TrimSpace(token) != "" {
		escTok := url.QueryEscape(strings.TrimSpace(token))
		tokParam = "&token=" + escTok
		panelTokParam = "token=" + escTok + "&"
	}

	const (
		uuidDetectLink = "11111111-1111-1111-1111-111111111111"
		uuidFirstItem  = "22222222-2222-2222-2222-222222222222"
		uuidURLEncode  = "33333333-3333-3333-3333-333333333333"
		uuidMenuGroup  = "44444444-4444-4444-4444-444444444444"
		uuidQuickMP4   = "55555555-5555-5555-5555-555555555551"
		uuidQuickMP3   = "55555555-5555-5555-5555-555555555552"
		uuidLocalMP4   = "55555555-5555-5555-5555-555555555553"
		uuidLocalMP3   = "55555555-5555-5555-5555-555555555554"

		optQuickMP4 = "⚡ 快速下载视频 (存电脑 1080p)"
		optQuickMP3 = "🎵 快速提取 MP3 (存电脑)"
		optLocalMP4 = "📲 下载视频到 iPhone 本地"
		optLocalMP3 = "🎧 下载 MP3 到 iPhone 本地"
		optWebPanel = "📂 选分类目录 / 打开 Web 面板"
	)

	quickMP4URL := renderTokenURLParam(base+"/api/quick?format=mp4&plain=1"+tokParam+"&url=", uuidURLEncode, "URL Encoded Text")
	quickMP3URL := renderTokenURLParam(base+"/api/quick?format=mp3&plain=1"+tokParam+"&url=", uuidURLEncode, "URL Encoded Text")
	localMP4URL := renderTokenURLParam(base+"/api/quick?format=mp4&wait=1"+tokParam+"&url=", uuidURLEncode, "URL Encoded Text")
	localMP3URL := renderTokenURLParam(base+"/api/quick?format=mp3&wait=1"+tokParam+"&url=", uuidURLEncode, "URL Encoded Text")
	panelOpenURL := renderTokenURLParam(base+"/?"+panelTokParam+"url=", uuidURLEncode, "URL Encoded Text")

	notifMP4Body := renderTokenURLParam("", uuidQuickMP4, "Contents of URL")
	notifMP3Body := renderTokenURLParam("", uuidQuickMP3, "Contents of URL")

	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>WFWorkflowActions</key>
	<array>
		<dict>
			<key>WFWorkflowActionIdentifier</key>
			<string>is.workflow.actions.detect.link</string>
			<key>WFWorkflowActionParameters</key>
			<dict>
				<key>UUID</key>
				<string>%s</string>
				<key>WFInput</key>
				<dict>
					<key>Value</key>
					<dict>
						<key>attachmentsByRange</key>
						<dict>
							<key>{0, 1}</key>
							<dict>
								<key>Type</key>
								<string>ExtensionInput</string>
							</dict>
						</dict>
						<key>string</key>
						<string>`+"\uFFFC"+`</string>
					</dict>
					<key>WFSerializationType</key>
					<string>WFTextTokenString</string>
				</dict>
			</dict>
		</dict>
		<dict>
			<key>WFWorkflowActionIdentifier</key>
			<string>is.workflow.actions.getitemfromlist</string>
			<key>WFWorkflowActionParameters</key>
			<dict>
				<key>UUID</key>
				<string>%s</string>
				<key>WFInput</key>
				<dict>
					<key>Value</key>
					<dict>
						<key>OutputName</key>
						<string>URLs</string>
						<key>OutputUUID</key>
						<string>%s</string>
						<key>Type</key>
						<string>ActionOutput</string>
					</dict>
					<key>WFSerializationType</key>
					<string>WFTextTokenAttachment</string>
				</dict>
				<key>WFItemSpecifier</key>
				<string>First Item</string>
			</dict>
		</dict>
		<dict>
			<key>WFWorkflowActionIdentifier</key>
			<string>is.workflow.actions.urlencode</string>
			<key>WFWorkflowActionParameters</key>
			<dict>
				<key>UUID</key>
				<string>%s</string>
				<key>WFEncodeMode</key>
				<string>Encode</string>
				<key>WFInput</key>
				%s
			</dict>
		</dict>
		<dict>
			<key>WFWorkflowActionIdentifier</key>
			<string>is.workflow.actions.choosefrommenu</string>
			<key>WFWorkflowActionParameters</key>
			<dict>
				<key>GroupingIdentifier</key>
				<string>%s</string>
				<key>WFControlFlowMode</key>
				<integer>0</integer>
				<key>WFMenuItems</key>
				<array>
					<string>%s</string>
					<string>%s</string>
					<string>%s</string>
					<string>%s</string>
					<string>%s</string>
				</array>
				<key>WFMenuPrompt</key>
				<string>📥 u2b 选择下载方式</string>
			</dict>
		</dict>
		<dict>
			<key>WFWorkflowActionIdentifier</key>
			<string>is.workflow.actions.choosefrommenu</string>
			<key>WFWorkflowActionParameters</key>
			<dict>
				<key>GroupingIdentifier</key>
				<string>%s</string>
				<key>WFControlFlowMode</key>
				<integer>1</integer>
				<key>WFMenuItemTitle</key>
				<string>%s</string>
			</dict>
		</dict>
		<dict>
			<key>WFWorkflowActionIdentifier</key>
			<string>is.workflow.actions.downloadurl</string>
			<key>WFWorkflowActionParameters</key>
			<dict>
				<key>UUID</key>
				<string>%s</string>
				<key>WFHTTPMethod</key>
				<string>GET</string>
				<key>WFURL</key>
				%s
			</dict>
		</dict>
		<dict>
			<key>WFWorkflowActionIdentifier</key>
			<string>is.workflow.actions.notification</string>
			<key>WFWorkflowActionParameters</key>
			<dict>
				<key>WFNotificationActionBody</key>
				%s
				<key>WFNotificationActionTitle</key>
				<string>📥 u2b 视频下载</string>
			</dict>
		</dict>
		<dict>
			<key>WFWorkflowActionIdentifier</key>
			<string>is.workflow.actions.choosefrommenu</string>
			<key>WFWorkflowActionParameters</key>
			<dict>
				<key>GroupingIdentifier</key>
				<string>%s</string>
				<key>WFControlFlowMode</key>
				<integer>1</integer>
				<key>WFMenuItemTitle</key>
				<string>%s</string>
			</dict>
		</dict>
		<dict>
			<key>WFWorkflowActionIdentifier</key>
			<string>is.workflow.actions.downloadurl</string>
			<key>WFWorkflowActionParameters</key>
			<dict>
				<key>UUID</key>
				<string>%s</string>
				<key>WFHTTPMethod</key>
				<string>GET</string>
				<key>WFURL</key>
				%s
			</dict>
		</dict>
		<dict>
			<key>WFWorkflowActionIdentifier</key>
			<string>is.workflow.actions.notification</string>
			<key>WFWorkflowActionParameters</key>
			<dict>
				<key>WFNotificationActionBody</key>
				%s
				<key>WFNotificationActionTitle</key>
				<string>🎵 u2b MP3 提取</string>
			</dict>
		</dict>
		<dict>
			<key>WFWorkflowActionIdentifier</key>
			<string>is.workflow.actions.choosefrommenu</string>
			<key>WFWorkflowActionParameters</key>
			<dict>
				<key>GroupingIdentifier</key>
				<string>%s</string>
				<key>WFControlFlowMode</key>
				<integer>1</integer>
				<key>WFMenuItemTitle</key>
				<string>%s</string>
			</dict>
		</dict>
		<dict>
			<key>WFWorkflowActionIdentifier</key>
			<string>is.workflow.actions.notification</string>
			<key>WFWorkflowActionParameters</key>
			<dict>
				<key>WFNotificationActionBody</key>
				<string>正在通过 u2b 解析下载，完成后会自动弹出保存窗口...</string>
				<key>WFNotificationActionTitle</key>
				<string>⏳ 视频准备中</string>
			</dict>
		</dict>
		<dict>
			<key>WFWorkflowActionIdentifier</key>
			<string>is.workflow.actions.downloadurl</string>
			<key>WFWorkflowActionParameters</key>
			<dict>
				<key>UUID</key>
				<string>%s</string>
				<key>WFHTTPMethod</key>
				<string>GET</string>
				<key>WFURL</key>
				%s
			</dict>
		</dict>
		<dict>
			<key>WFWorkflowActionIdentifier</key>
			<string>is.workflow.actions.documentpicker.save</string>
			<key>WFWorkflowActionParameters</key>
			<dict>
				<key>WFAskWhereToSave</key>
				<true/>
				<key>WFInput</key>
				<dict>
					<key>Value</key>
					<dict>
						<key>OutputName</key>
						<string>Contents of URL</string>
						<key>OutputUUID</key>
						<string>%s</string>
						<key>Type</key>
						<string>ActionOutput</string>
					</dict>
					<key>WFSerializationType</key>
					<string>WFTextTokenAttachment</string>
				</dict>
			</dict>
		</dict>
		<dict>
			<key>WFWorkflowActionIdentifier</key>
			<string>is.workflow.actions.choosefrommenu</string>
			<key>WFWorkflowActionParameters</key>
			<dict>
				<key>GroupingIdentifier</key>
				<string>%s</string>
				<key>WFControlFlowMode</key>
				<integer>1</integer>
				<key>WFMenuItemTitle</key>
				<string>%s</string>
			</dict>
		</dict>
		<dict>
			<key>WFWorkflowActionIdentifier</key>
			<string>is.workflow.actions.notification</string>
			<key>WFWorkflowActionParameters</key>
			<dict>
				<key>WFNotificationActionBody</key>
				<string>正在通过 u2b 提取高音质 MP3，完成后会自动弹出保存窗口...</string>
				<key>WFNotificationActionTitle</key>
				<string>⏳ MP3 准备中</string>
			</dict>
		</dict>
		<dict>
			<key>WFWorkflowActionIdentifier</key>
			<string>is.workflow.actions.downloadurl</string>
			<key>WFWorkflowActionParameters</key>
			<dict>
				<key>UUID</key>
				<string>%s</string>
				<key>WFHTTPMethod</key>
				<string>GET</string>
				<key>WFURL</key>
				%s
			</dict>
		</dict>
		<dict>
			<key>WFWorkflowActionIdentifier</key>
			<string>is.workflow.actions.documentpicker.save</string>
			<key>WFWorkflowActionParameters</key>
			<dict>
				<key>WFAskWhereToSave</key>
				<true/>
				<key>WFInput</key>
				<dict>
					<key>Value</key>
					<dict>
						<key>OutputName</key>
						<string>Contents of URL</string>
						<key>OutputUUID</key>
						<string>%s</string>
						<key>Type</key>
						<string>ActionOutput</string>
					</dict>
					<key>WFSerializationType</key>
					<string>WFTextTokenAttachment</string>
				</dict>
			</dict>
		</dict>
		<dict>
			<key>WFWorkflowActionIdentifier</key>
			<string>is.workflow.actions.choosefrommenu</string>
			<key>WFWorkflowActionParameters</key>
			<dict>
				<key>GroupingIdentifier</key>
				<string>%s</string>
				<key>WFControlFlowMode</key>
				<integer>1</integer>
				<key>WFMenuItemTitle</key>
				<string>%s</string>
			</dict>
		</dict>
		<dict>
			<key>WFWorkflowActionIdentifier</key>
			<string>is.workflow.actions.openurl</string>
			<key>WFWorkflowActionParameters</key>
			<dict>
				<key>WFInput</key>
				%s
			</dict>
		</dict>
		<dict>
			<key>WFWorkflowActionIdentifier</key>
			<string>is.workflow.actions.choosefrommenu</string>
			<key>WFWorkflowActionParameters</key>
			<dict>
				<key>GroupingIdentifier</key>
				<string>%s</string>
				<key>WFControlFlowMode</key>
				<integer>2</integer>
			</dict>
		</dict>
	</array>
	<key>WFWorkflowClientVersion</key>
	<string>1200</string>
	<key>WFWorkflowHasOutputFallback</key>
	<false/>
	<key>WFWorkflowHasShortcutInputVariables</key>
	<true/>
	<key>WFWorkflowIcon</key>
	<dict>
		<key>WFWorkflowIconGlyphNumber</key>
		<integer>59771</integer>
		<key>WFWorkflowIconStartColor</key>
		<integer>463140863</integer>
	</dict>
	<key>WFWorkflowImportQuestions</key>
	<array/>
	<key>WFWorkflowInputContentItemClasses</key>
	<array>
		<string>WFURLContentItem</string>
		<string>WFSafariWebPageContentItem</string>
		<string>WFStringContentItem</string>
		<string>WFArticleContentItem</string>
		<string>WFRichTextContentItem</string>
	</array>
	<key>WFWorkflowMinimumClientVersion</key>
	<integer>900</integer>
	<key>WFWorkflowMinimumClientVersionString</key>
	<string>900</string>
	<key>WFWorkflowName</key>
	<string>u2b 下载</string>
	<key>WFWorkflowNoInputBehavior</key>
	<dict>
		<key>Name</key>
		<string>WFWorkflowNoInputBehaviorGetClipboard</string>
		<key>Parameters</key>
		<dict/>
	</dict>
	<key>WFWorkflowOutputContentItemClasses</key>
	<array/>
	<key>WFWorkflowTypes</key>
	<array>
		<string>ActionExtension</string>
		<string>NCWidget</string>
	</array>
</dict>
</plist>
`,
		uuidDetectLink,
		uuidFirstItem, uuidDetectLink,
		uuidURLEncode, renderTokenURLParam("", uuidFirstItem, "Item from List"),
		uuidMenuGroup, optQuickMP4, optQuickMP3, optLocalMP4, optLocalMP3, optWebPanel,
		// Option 1: Quick MP4 to Mac
		uuidMenuGroup, optQuickMP4,
		uuidQuickMP4, quickMP4URL,
		notifMP4Body,
		// Option 2: Quick MP3 to Mac
		uuidMenuGroup, optQuickMP3,
		uuidQuickMP3, quickMP3URL,
		notifMP3Body,
		// Option 3: Local MP4 to iPhone
		uuidMenuGroup, optLocalMP4,
		uuidLocalMP4, localMP4URL,
		uuidLocalMP4,
		// Option 4: Local MP3 to iPhone
		uuidMenuGroup, optLocalMP3,
		uuidLocalMP3, localMP3URL,
		uuidLocalMP3,
		// Option 5: Open Web Panel
		uuidMenuGroup, optWebPanel,
		panelOpenURL,
		// End Menu
		uuidMenuGroup,
	)
}
