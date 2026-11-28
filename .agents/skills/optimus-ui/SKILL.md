---
name: optimus-ui
description: "Offline Optimus UI (@openng/optimus-ui v2.1.0) component APIs, templates, pass-through options, CSS classes, and theming tokens. Use for p-* components, p* directives, or @openng/optimus-ui imports."
---

# Optimus UI docs (local copy)

Optimus UI is a community-maintained, MIT-licensed continuation of PrimeNG: 80+ accessible Angular components, targeting Angular v21+. This skill bundles the documentation published at <https://optimus.openng.org/llms/llms.txt> (generated from Optimus UI v2.1.0), so you can read it from disk instead of fetching `https://optimus.openng.org/llms/components/<name>.md` each time.

All paths below are relative to this skill's directory (the folder containing this SKILL.md).

## How to use the docs

1. Find the file in the index below. The file name is the component's docs slug (`select`, `datepicker`, `inputotp`, `galleria`, `scroller`, ...); each line also lists the selectors the page documents, so search the index for a selector such as `pTooltip` or `p-columnFilter`.
2. Read only the part you need. Component files are 10–60 KB, so list the headings first and then read the matching line range, for example `grep -n '^##' references/components/table.md`.
3. When an exact API detail matters (input name, allowed values, event payload, template context, token name), copy it from the docs instead of relying on memory of PrimeNG. Optimus UI is API-compatible with PrimeNG v21 in most places, but names, packages and defaults can differ.

To search everything at once, grep the folder, for example `grep -rn "appendTo" references/components/`.

### Layout of a component file

- `# <Name>` followed by a one-line summary.
- `## Import`: the exact import statements for the page's modules, standalone components and services.
- One `##` section per demo (Basic, Template, Sizes, Disabled, ...). Each has a short explanation and a full standalone Angular component in a `typescript` block.
- `## Accessibility`: screen-reader and keyboard support.
- `## API`: one `### <ClassName>` per component or directive with its selector, whether it works with `ngModel`/reactive forms, and `#### Inputs` (type or allowed values, default, two-way bindings), `#### Outputs` (payload type), `#### Templates` (context type) and `#### Methods`. This is generated from the compiled library and is the authoritative API.
- `## Pass Through Options`: the `pt` keys for styling internal elements.
- `## Theming`, with `### CSS Classes` and `### Design Tokens` (token name and CSS variable).

## Conventions that hold across the library

Check the guides when in doubt; these are here so simple tasks don't need a lookup:

- Packages: `@openng/optimus-ui` (one secondary entry point per component, such as `@openng/optimus-ui/button`) and `@openng/optimus-ui-themes` (theme presets).
- Components are imported per entry point (`import { ButtonModule } from '@openng/optimus-ui/button'`) into standalone components. The docs examples show the exact symbol to import, so copy it from there.
- App-wide setup (theme preset, ripple, z-index, locale and so on) goes through `provideOptimus({...})` from `@openng/optimus-ui/config`; see `references/guides/configuration.md`. New projects can use `ng add @openng/optimus-ui`. Existing PrimeNG apps use the `migrate-from-primeng` schematic instead (see `references/guides/installation.md` and `references/guides/faq.md`).
- Selectors use the `p-` prefix (`<p-button>`, `<p-table>`) and directives the `p` prefix (`pButton`, `pTooltip`, `pInputText`).

## Keeping the docs up to date

These docs are a snapshot of v2.1.0. If the project uses a newer Optimus UI release, or the docs contradict the installed package, offer to update it by running `npx skills update optimus-ui`, or `npx skills add https://optimus.openng.org` (add `-g` for a user-level install) if it was installed another way. Any single page can also be fetched live from `https://optimus.openng.org/llms/components/<name>.md` or `https://optimus.openng.org/llms/pages/<name>.md`, and the whole API as JSON from `https://optimus.openng.org/llms/manifest.json`. For live lookups inside the agent, the `@openng/optimus-ui-mcp` MCP server serves the same data.

## Index

### Guides (`references/guides/`)

- `references/guides/accessibility.md` — **Accessibility**: Optimus UI has WCAG 2.1 AA level compliance.
- `references/guides/animations.md` — **Animations**: Built-in CSS animations for Optimus UI components.
- `references/guides/configuration.md` — **Configuration**: Application wide configuration for Optimus UI.
- `references/guides/contribution.md` — **Contribution Guide**: How to contribute to Optimus UI.
- `references/guides/customicons.md` — **Custom Icons**: Use custom icons with Optimus UI components.
- `references/guides/faq.md` — **FAQ**: Licensing, migration from PrimeNG, the ecosystem, and how support works.
- `references/guides/icons.md` — **Icons**: OpenNG Icons is the default icon library of Optimus UI with over 250 open source icons.
- `references/guides/installation.md` — **Installation**: Setting up Optimus UI in an Angular CLI project.
- `references/guides/llms.md` — **LLMs.txt**: LLM-optimized documentation endpoints for Optimus UI components.
- `references/guides/passthrough.md` — **Pass Through**: Pass Through Props allow direct access to the underlying elements for complete customization.
- `references/guides/philosophy.md` — **Philosophy**: Why Optimus UI exists, what it commits to, and where it stops.
- `references/guides/primeflex.md` — **PrimeFlex**: Moving from PrimeFlex to Tailwind CSS.
- `references/guides/rtl.md` — **RTL**: Right-to-left support for Optimus UI components.
- `references/guides/styled.md` — **Styled Mode**: Choose from a variety of pre-styled themes or develop your own.
- `references/guides/tailwind.md` — **Tailwind CSS**: Integration between Optimus UI and Tailwind CSS.
- `references/guides/unstyled.md` — **Unstyled Mode**: Theming Optimus UI with alternative styling approaches.

### Components (`references/components/`)

- `references/components/accordion.md` — **Accordion** (`p-accordion`, `p-accordion-content`, `p-accordion-header`, `p-accordion-panel`): Accordion groups a collection of contents in tabs.
- `references/components/animateonscroll.md` — **Animate On Scroll** (`[pAnimateOnScroll]`): AnimateOnScroll is used to apply animations to elements when entering or leaving the viewport during scrolling.
- `references/components/autocomplete.md` — **AutoComplete** (`p-autoComplete`): AutoComplete is an input component that provides real-time suggestions when being typed.
- `references/components/autofocus.md` — **AutoFocus** (`[pAutoFocus]`): AutoFocus manages focus on focusable element on load.
- `references/components/avatar.md` — **Avatar** (`p-avatar`, `p-avatarGroup`): Avatar represents people using icons, labels and images.
- `references/components/badge.md` — **Badge** (`p-badge`, `[pBadge]`): Badge is a small status indicator for another element.
- `references/components/blockui.md` — **BlockUI** (`p-blockUI`): BlockUI can either block other components or the whole page.
- `references/components/breadcrumb.md` — **Breadcrumb** (`p-breadcrumb`): Breadcrumb provides contextual information about page hierarchy.
- `references/components/button.md` — **Button** (`p-button`, `[pButton]`, `[pButtonIcon]`, `[pButtonLabel]`): Button is an extension to standard button element with icons and theming.
- `references/components/card.md` — **Card** (`p-card`): Card is a flexible container component.
- `references/components/carousel.md` — **Carousel** (`p-carousel`): Carousel is a content slider featuring various customization options.
- `references/components/cascadeselect.md` — **CascadeSelect** (`p-cascadeSelect`, `ul[pCascadeSelectSub]`): CascadeSelect displays a nested structure of options.
- `references/components/chart.md` — **Chart** (`p-chart`): Chart components are based on Charts.js 3.3.2+, an open source HTML5 based charting library.
- `references/components/checkbox.md` — **Checkbox** (`p-checkbox`): Checkbox is an extension to standard checkbox element with theming.
- `references/components/chip.md` — **Chip** (`p-chip`): Chip represents entities using icons, labels and images.
- `references/components/colorpicker.md` — **ColorPicker** (`p-colorPicker`): ColorPicker is an input component to select a color.
- `references/components/confirmdialog.md` — **ConfirmDialog** (`p-confirmDialog`): ConfirmDialog is backed by a service utilizing Observables to display confirmation windows easily that can be shared by multiple actions on the same component.
- `references/components/confirmpopup.md` — **ConfirmPopup** (`p-confirmpopup`): ConfirmPopup displays a confirmation overlay displayed relatively to its target.
- `references/components/contextmenu.md` — **ContextMenu** (`p-contextMenu`, `p-contextMenuSub`): ContextMenu displays an overlay menu on right click of its target.
- `references/components/dataview.md` — **DataView** (`p-dataView`): DataView displays data in grid grid-cols-12 gap-4 or list layout with pagination and sorting features.
- `references/components/datepicker.md` — **DatePicker** (`p-datePicker`): DatePicker is an input component to select a date.
- `references/components/dialog.md` — **Dialog** (`p-dialog`): Dialog is a container to display content in an overlay window.
- `references/components/divider.md` — **Divider** (`p-divider`): Divider is used to separate contents.
- `references/components/dock.md` — **Dock** (`p-dock`): Dock is a navigation component consisting of menuitems.
- `references/components/dragdrop.md` — **Drag and Drop** (`[pDraggable]`, `[pDroppable]`): pDraggable and pDroppable directives apply drag-drop behaviors to any element.
- `references/components/drawer.md` — **Drawer** (`p-drawer`): Drawer is a container component displayed as an overlay.
- `references/components/dynamicdialog.md` — **Dynamic Dialog** (`p-dynamicDialog`): Dialogs can be created dynamically with any component as the content using a DialogService.
- `references/components/editor.md` — **Editor** (`p-editor`): Editor is rich text editor component based on Quill.
- `references/components/fieldset.md` — **Fieldset** (`p-fieldset`): Fieldset is a grouping component with a content toggle feature.
- `references/components/fileupload.md` — **FileUpload** (`[pFileContent]`, `p-fileupload`): FileUpload is an advanced uploader with dragdrop support, multi file uploads, auto uploading, progress tracking and validations.
- `references/components/floatlabel.md` — **Float Label** (`p-floatlabel`): FloatLabel appears on top of the input field when focused.
- `references/components/fluid.md` — **Fluid** (`p-fluid`): Fluid is a layout component to make descendant components span full width of their container.
- `references/components/focustrap.md` — **Focus Trap** (`[pFocusTrap]`): Focus Trap keeps focus within a certain DOM element while tabbing.
- `references/components/galleria.md` — **Gallery** (`p-galleria`, `div[pGalleriaContent]`, `div[pGalleriaItem]`, `div[pGalleriaItemSlot]`, `div[pGalleriaThumbnails]`): Galleria is an advanced content gallery component.
- `references/components/iconfield.md` — **IconField** (`p-iconfield`, `p-inputicon`): IconField wraps an input and an icon.
- `references/components/iftalabel.md` — **Ifta Label** (`p-iftalabel`): IftaLabel is used to create infield top aligned labels.
- `references/components/image.md` — **Image** (`p-image`): Displays an image with preview and tranformation options.
- `references/components/imagecompare.md` — **ImageCompare** (`p-imageCompare`): Compare two images side by side with a slider.
- `references/components/inplace.md` — **Inplace** (`p-inplace`, `p-inplacecontent`, `p-inplacedisplay`): Inplace provides an easy to do editing and display at the same time where clicking the output displays the actual content.
- `references/components/inputgroup.md` — **InputGroup** (`p-inputgroup`): Text, icon, buttons and other content can be grouped next to an input.
- `references/components/inputmask.md` — **InputMask** (`p-inputmask`, `[pInputMask]`): InputMask component is used to enter input in a certain format such as numeric, date, currency and phone.
- `references/components/inputnumber.md` — **InputNumber** (`p-inputNumber`): InputNumber is an input component to provide numerical input.
- `references/components/inputotp.md` — **Otp Input** (`p-inputOtp`): Input Otp is used to enter one time passwords.
- `references/components/inputtext.md` — **InputText** (`[pInputText]`): InputText is an extension to standard input element with theming and keyfiltering.
- `references/components/keyfilter.md` — **KeyFilter** (`[pKeyFilter]`): KeyFilter is a directive to restrict individual key strokes. In order to restrict the whole input, use InputNumber or InputMask instead.
- `references/components/knob.md` — **Knob** (`p-knob`): Knob is a form component to define number inputs with a dial.
- `references/components/listbox.md` — **Listbox** (`p-listbox`): Listbox is used to select one or more values from a list of items.
- `references/components/megamenu.md` — **MegaMenu** (`p-megaMenu`, `p-megaMenuSub`): MegaMenu is navigation component that displays submenus together.
- `references/components/menu.md` — **Menu** (`p-menu`, `[pMenuItemContent]`): Menu is a navigation / command component that supports dynamic and static positioning.
- `references/components/menubar.md` — **Menubar** (`p-menubar`, `p-menubarSub`): Menubar is a horizontal menu component.
- `references/components/message.md` — **Message** (`p-message`): Message component is used to display inline messages.
- `references/components/metergroup.md` — **MeterGroup** (`p-meterGroup`, `p-meterGroupLabel`): MeterGroup displays scalar measurements within a known range.
- `references/components/multiselect.md` — **MultiSelect** (`p-multiSelect`, `li[pMultiSelectItem]`): MultiSelect is used to select multiple items from a collection.
- `references/components/orderlist.md` — **OrderList** (`p-orderList`): OrderList is used to sort a collection.
- `references/components/organizationchart.md` — **Organization Chart** (`p-organizationChart`, `[pOrganizationChartNode]`): OrganizationChart visualizes hierarchical organization data.
- `references/components/overlay.md` — **Overlay API** (`p-overlay`): This API allows overlay components to be controlled from the Optimus UI. In this way, all overlay components in the application can have the same behavior.
- `references/components/paginator.md` — **Paginator** (`p-paginator`): Paginator displays data in paged format and provides navigation between pages.
- `references/components/panel.md` — **Panel** (`p-panel`): Panel is a container component with an optional content toggle feature.
- `references/components/panelmenu.md` — **PanelMenu** (`p-panelMenu`, `ul[pPanelMenuList]`, `ul[pPanelMenuSub]`): PanelMenu is a hybrid of Accordion and Tree components.
- `references/components/password.md` — **Password** (`p-password`, `[pPassword]`): Password displays strength indicator for password fields.
- `references/components/picklist.md` — **PickList** (`p-pickList`): PickList is used to reorder items between different lists.
- `references/components/popover.md` — **Popover** (`p-popover`): Popover is a container component that can overlay other components on page.
- `references/components/progressbar.md` — **ProgressBar** (`p-progressBar`): ProgressBar is a process status indicator.
- `references/components/progressspinner.md` — **ProgressSpinner** (`p-progressSpinner`): ProgressSpinner is a process status indicator.
- `references/components/radiobutton.md` — **RadioButton** (`p-radioButton`): RadioButton is an extension to standard radio button element with theming.
- `references/components/rating.md` — **Rating** (`p-rating`): Rating component is a star based selection input.
- `references/components/ripple.md` — **Ripple** (`[pRipple]`): Ripple directive adds ripple effect to the host element.
- `references/components/scroller.md` — **Virtual Scroller** (`p-scroller`): VirtualScroller is a performance-approach to handle huge data efficiently.
- `references/components/scrollpanel.md` — **Scroll Panel** (`p-scroll-panel`): ScrollPanel is a cross browser, lightweight and skinnable alternative to native browser scrollbar.
- `references/components/scrolltop.md` — **Scroll Top** (`p-scrollTop`): ScrollTop gets displayed after a certain scroll position and used to navigates to the top of the page quickly.
- `references/components/select.md` — **Select** (`p-select`, `p-selectItem`): Select is used to choose an item from a collection of options.
- `references/components/selectbutton.md` — **SelectButton** (`p-selectButton`): SelectButton is used to choose single or multiple items from a list using buttons.
- `references/components/skeleton.md` — **Skeleton** (`p-skeleton`): Skeleton is a placeholder to display instead of the actual content.
- `references/components/slider.md` — **Slider** (`p-slider`): Slider is a component to provide input with a drag handle.
- `references/components/speeddial.md` — **Speed Dial** (`p-speeddial`): SpeedDial is a floating button with a popup menu.
- `references/components/splitbutton.md` — **SplitButton** (`p-splitbutton`): SplitButton groups a set of commands in an overlay with a default action item.
- `references/components/splitter.md` — **Splitter** (`p-splitter`): Splitter is utilized to separate and resize panels.
- `references/components/stepper.md` — **Stepper** (`p-step`, `p-step-item`, `p-step-list`, `p-step-panel`, `p-step-panels`, `p-stepper`, `p-stepper-separator`): The Stepper component displays a wizard-like workflow by guiding users through the multi-step progression.
- `references/components/styleclass.md` — **StyleClass** (`[pStyleClass]`): StyleClass manages css classes declaratively to during enter/leave animations or just to toggle classes on an element.
- `references/components/table.md` — **Table** (`[pCancelEditableRow]`, `p-cellEditor`, `p-columnFilter`, `p-columnFilterFormElement`, `[pContextMenuRow]`, `[pEditableColumn]`, `[pEditableRow]`, `[pFrozenColumn]`, `[pInitEditableRow]`, `[pReorderableColumn]`, `[pReorderableRow]`, `[pReorderableRowHandle]`, `[pResizableColumn]`, `[pRowGroupHeader]`, `[pRowToggler]`, `[pSaveEditableRow]`, `[pSelectableRow]`, `[pSelectableRowDblClick]`, `p-sortIcon`, `[pSortableColumn]`, `p-table`, `[pTableBody]`, `p-tableCheckbox`, `p-tableHeaderCheckbox`, `p-tableRadioButton`): Table displays data in tabular format.
- `references/components/tabs.md` — **Tabs** (`p-tab`, `p-tablist`, `p-tabpanel`, `p-tabpanels`, `p-tabs`): Tabs is a container component to group content with tabs.
- `references/components/tag.md` — **Tag** (`p-tag`): Tag component is used to categorize content.
- `references/components/terminal.md` — **Terminal** (`p-terminal`): -
- `references/components/textarea.md` — **Textarea** (`[pTextarea]`): Textarea adds styling and autoResize functionality to standard textarea element.
- `references/components/tieredmenu.md` — **TieredMenu** (`p-tieredMenu`, `p-tieredMenuSub`): TieredMenu displays submenus in nested overlays.
- `references/components/timeline.md` — **Timeline** (`p-timeline`): Timeline visualizes a series of chained events.
- `references/components/toast.md` — **Toast** (`p-toast`, `p-toastItem`): Toast is used to display messages in an overlay.
- `references/components/togglebutton.md` — **ToggleButton** (`p-toggleButton`): ToggleButton is used to select a boolean value using a button.
- `references/components/toggleswitch.md` — **ToggleSwitch** (`p-toggleswitch`): ToggleSwitch is used to select a boolean value.
- `references/components/toolbar.md` — **Toolbar** (`p-toolbar`): Toolbar is a grouping component for buttons and other content.
- `references/components/tooltip.md` — **Tooltip** (`[pTooltip]`): Tooltip directive provides advisory information for a component. Tooltip is integrated within various Optimus UI components.
- `references/components/tree.md` — **Tree** (`p-tree`, `p-treeNode`): Tree is used to display hierarchical data.
- `references/components/treeselect.md` — **TreeSelect** (`p-treeSelect`): TreeSelect is a form component to choose from hierarchical data.
- `references/components/treetable.md` — **TreeTable** (`[pTreeTableBody]`, `p-treeTableCheckbox`, `[ttContextMenuRow]`, `[ttEditableColumn]`, `p-treeTableHeaderCheckbox`, `[ttReorderableColumn]`, `[ttResizableColumn]`, `[ttRow]`, `[ttScrollableView]`, `[ttSelectableRow]`, `[ttSelectableRowDblClick]`, `p-treeTableSortIcon`, `[ttSortableColumn]`, `p-treeTable`, `p-treeTableCellEditor`, `p-treeTableToggler`): TreeTable is used to display hierarchical data in tabular format.
