//go:build windows

package a11y

// The identifiers below are UI Automation's, copied from the mingw-w64
// headers rather than remembered: uiautomationclient.h for the numeric
// identifiers and uiautomationcore.h for the enumerations and interface
// IIDs. A wrong digit in any of them does not fail loudly -- UIA answers
// E_NOINTERFACE, or silently ignores a property -- so they are gathered
// here, in one place, spelled the way the headers spell them.
//
//	https://github.com/mirror/mingw-w64/blob/master/mingw-w64-headers/include/uiautomationclient.h
//	https://github.com/mirror/mingw-w64/blob/master/mingw-w64-headers/include/uiautomationcore.h

// Control types are what UIA calls a role. There is no switch and no
// heading among them, which is why axControlType also returns a localized
// name for the few roles that need one.
const (
	uiaButtonControlType      = 50000
	uiaCheckBoxControlType    = 50002
	uiaComboBoxControlType    = 50003
	uiaEditControlType        = 50004
	uiaHyperlinkControlType   = 50005
	uiaImageControlType       = 50006
	uiaListItemControlType    = 50007
	uiaListControlType        = 50008
	uiaMenuControlType        = 50009
	uiaMenuItemControlType    = 50011
	uiaProgressBarControlType = 50012
	uiaRadioButtonControlType = 50013
	uiaSliderControlType      = 50015
	uiaTabControlType         = 50018
	uiaTabItemControlType     = 50019
	uiaTextControlType        = 50020
	uiaToolBarControlType     = 50021
	uiaGroupControlType       = 50026
	uiaDataItemControlType    = 50029
	uiaWindowControlType      = 50032
	uiaPaneControlType        = 50033
	uiaSeparatorControlType   = 50038
)

// Property identifiers, for GetPropertyValue and for naming what changed in
// a property-changed event.
const (
	uiaBoundingRectangleProperty       = 30001
	uiaControlTypeProperty             = 30003
	uiaLocalizedControlTypeProperty    = 30004
	uiaNameProperty                    = 30005
	uiaHasKeyboardFocusProperty        = 30008
	uiaIsKeyboardFocusableProperty     = 30009
	uiaIsEnabledProperty               = 30010
	uiaAutomationIDProperty            = 30011
	uiaHelpTextProperty                = 30013
	uiaIsControlElementProperty        = 30016
	uiaIsContentElementProperty        = 30017
	uiaIsOffscreenProperty             = 30022
	uiaOrientationProperty             = 30023
	uiaValueValueProperty              = 30045
	uiaRangeValueValueProperty         = 30047
	uiaExpandCollapseStateProperty     = 30070
	uiaSelectionItemIsSelectedProperty = 30079
	uiaToggleToggleStateProperty       = 30086
	uiaLevelProperty                   = 30154
)

// Pattern identifiers, for GetPatternProvider.
const (
	uiaInvokePatternID         = 10000
	uiaValuePatternID          = 10002
	uiaRangeValuePatternID     = 10003
	uiaExpandCollapsePatternID = 10005
	uiaSelectionItemPatternID  = 10010
	uiaTogglePatternID         = 10015
)

// Event identifiers, for the notifications a frame's diff turns into.
const (
	uiaStructureChangedEvent          = 20002
	uiaAutomationPropertyChangedEvent = 20004
	uiaAutomationFocusChangedEvent    = 20005
	uiaNotificationEvent              = 20035
)

// ProviderOptions says what kind of provider this is. ServerSideProvider
// alone, without UseComThreading, means UIA calls these methods on its own
// RPC threads rather than marshalling them to an apartment. That is what
// the bridge is already built for -- the published frame is behind an
// atomic pointer and the element cache behind a mutex -- and it is why no
// provider method below may touch Ebitengine, which is not thread-safe.
const uiaProviderOptionsServerSide = 0x2

// NavigateDirection, for IRawElementProviderFragment::Navigate.
const (
	uiaNavigateParent = iota
	uiaNavigateNextSibling
	uiaNavigatePreviousSibling
	uiaNavigateFirstChild
	uiaNavigateLastChild
)

// StructureChangeType, for a layout change.
const uiaStructureChildrenInvalidated = 2

// ToggleState, for a check box, radio button or switch.
const (
	uiaToggleOff = iota
	uiaToggleOn
	uiaToggleIndeterminate
)

// ExpandCollapseState, for a disclosure, accordion or combobox.
const (
	uiaCollapsed = iota
	uiaExpanded
	uiaPartiallyExpanded
	uiaLeafNode
)

// NotificationKind and NotificationProcessing, for Announce.
const (
	uiaNotificationKindOther              = 4
	uiaNotificationProcessingAll          = 2
	uiaNotificationProcessingImportantAll = 0
)

// OrientationType, for a slider and a separator.
const (
	uiaOrientationNone = iota
	uiaOrientationHorizontal
	uiaOrientationVertical
)

// uiaRootObjectID is the lParam WM_GETOBJECT carries when the caller wants
// the window's UI Automation provider, and uiaAppendRuntimeID is the
// leading element of a runtime identifier a fragment supplies the tail of.
const (
	uiaRootObjectID    = -25
	uiaAppendRuntimeID = 3
)

// axControlType maps a ggui role onto the UIA control type an element
// reports, and onto the localized name it reports alongside it for the
// roles UIA has no type for. An empty name means the element lets UIA
// supply the usual one for its type, which is localized for the user's
// language and is almost always the right answer.
//
// A switch is a check box, because UIA has no switch and a screen reader
// reads a check box correctly; only its spoken type is overridden. A
// heading is text with a level, which is how UIA expresses one. A tab is a
// tab item inside a tab control, which is the pairing UIA expects.
func axControlType(r Role) (ctl int32, localized string) {
	switch r {
	case RoleButton:
		return uiaButtonControlType, ""
	case RoleCheckbox:
		return uiaCheckBoxControlType, ""
	case RoleRadio:
		return uiaRadioButtonControlType, ""
	case RoleSwitch:
		return uiaCheckBoxControlType, "switch"
	case RoleSlider:
		return uiaSliderControlType, ""
	case RoleTextField:
		return uiaEditControlType, ""
	case RoleSelect, RoleCombobox:
		return uiaComboBoxControlType, ""
	case RoleOption:
		return uiaListItemControlType, ""
	case RoleMenu:
		return uiaMenuControlType, ""
	case RoleMenuItem:
		return uiaMenuItemControlType, ""
	case RoleTab:
		return uiaTabItemControlType, ""
	case RoleTabs:
		return uiaTabControlType, ""
	case RoleDisclosure:
		return uiaButtonControlType, "disclosure triangle"
	case RoleAccordion:
		return uiaGroupControlType, "accordion"
	case RoleDialog:
		return uiaWindowControlType, "dialog"
	case RoleRow:
		return uiaDataItemControlType, ""
	case RoleListItem:
		return uiaListItemControlType, ""
	case RoleSeparator:
		return uiaSeparatorControlType, ""
	case RoleText:
		return uiaTextControlType, ""
	case RoleHeading:
		return uiaTextControlType, "heading"
	case RoleImage:
		return uiaImageControlType, ""
	case RoleList:
		return uiaListControlType, ""
	case RoleProgress:
		return uiaProgressBarControlType, ""
	case RoleLink:
		return uiaHyperlinkControlType, ""
	case RoleToolbar:
		return uiaToolBarControlType, ""
	case RoleStatus:
		return uiaGroupControlType, "status"
	case RoleWindow:
		return uiaWindowControlType, ""
	case RoleGroup:
		return uiaGroupControlType, ""
	}
	return uiaPaneControlType, ""
}
