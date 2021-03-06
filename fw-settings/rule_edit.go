package main

import (
	"fmt"
	"github.com/gotk3/gotk3/gtk"
	"net"
	"strconv"
	"strings"
	"unicode"
)

const (
	editDialogCancel = 1
	editDialogOk     = 2
)

type ruleEdit struct {
	row       *ruleRow
	dialog    *gtk.Dialog
	pathLabel *gtk.Label
	verbCombo *gtk.ComboBoxText
	hostEntry *gtk.Entry
	portEntry *gtk.Entry
	ok        *gtk.Button
}

func newRuleEdit(rr *ruleRow) *ruleEdit {
	redit := &ruleEdit{row: rr}
	b := newBuilder("RuleEdit")
	b.getItems(
		"dialog", &redit.dialog,
		"path_label", &redit.pathLabel,
		"verb_combo", &redit.verbCombo,
		"host_entry", &redit.hostEntry,
		"port_entry", &redit.portEntry,
		"ok_button", &redit.ok,
	)
	b.ConnectSignals(map[string]interface{}{
		"on_port_insert_text": redit.onPortInsertText,
		"on_port_changed":     redit.onChanged,
		"on_host_changed":     redit.onChanged,
	})
	return redit
}

func (re *ruleEdit) updateDialogFields() {
	r := re.row.rule
	re.pathLabel.SetText(r.Path)
	if r.Verb == RULE_ALLOW {
		re.verbCombo.SetActiveID("allow")
	} else {
		re.verbCombo.SetActiveID("deny")
	}
	target := strings.Split(r.Target, ":")
	if len(target) != 2 {
		return
	}
	re.hostEntry.SetText(target[0])
	re.portEntry.SetText(target[1])
}

func (re *ruleEdit) validateFields() bool {
	id := re.verbCombo.GetActiveID()
	if id != "allow" && id != "deny" {
		return false
	}
	host, _ := re.hostEntry.GetText()
	port, _ := re.portEntry.GetText()
	if !isValidHost(host) {
		return false
	}
	if !isValidPort(port) {
		return false
	}
	return true
}

func isValidHost(host string) bool {
	if host == "*" {
		return true
	}
	if net.ParseIP(host) != nil {
		return true
	}

	parts := strings.Split(host, ".")
	if len(parts) < 2 {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
	}
	return true
}

func isValidPort(port string) bool {
	if port == "*" {
		return true
	}

	pval, err := strconv.Atoi(port)
	if err != nil {
		return false
	}
	return pval > 0 && pval <= 0xFFFF
}

func (re *ruleEdit) updateRow() {
	if !re.validateFields() {
		return
	}
	r := re.row.rule
	switch re.verbCombo.GetActiveID() {
	case "allow":
		r.Verb = RULE_ALLOW
	case "deny":
		r.Verb = RULE_DENY
	}
	host, _ := re.hostEntry.GetText()
	port, _ := re.portEntry.GetText()
	r.Target = fmt.Sprintf("%s:%s", host, port)
	re.row.update()
}

func (re *ruleEdit) run() {
	re.dialog.SetTransientFor(re.row.rl.win)
	if re.dialog.Run() == editDialogOk {
		re.updateRow()
		re.row.rl.dbus.updateRule(re.row.rule)
	}
	re.dialog.Destroy()
}

func (rr *ruleRow) runEditor() {
	redit := newRuleEdit(rr)
	redit.updateDialogFields()
	redit.run()
}

func (re *ruleEdit) onPortInsertText(entry *gtk.Entry, text string) {
	current, _ := entry.GetText()
	if current == "" && text == "*" {
		return
	}
	if current == "*" {
		entry.StopEmission("insert-text")
		return
	}
	for _, c := range text {
		if !unicode.IsDigit(c) {
			entry.StopEmission("insert-text")
			return
		}
	}
}

func (re *ruleEdit) onChanged() {
	re.ok.SetSensitive(re.validateFields())
}
