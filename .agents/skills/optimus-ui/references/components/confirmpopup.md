# ConfirmPopup

ConfirmPopup displays a confirmation overlay displayed relatively to its target.

## Import

```typescript
import { ConfirmPopupModule, ConfirmPopup } from '@openng/optimus-ui/confirmpopup';
import { Confirmation, ConfirmationService } from '@openng/optimus-ui/api';
```

## Basic

ConfirmPopup is defined using p-confirmPopup tag and an instance of ConfirmationService is required to display it bycalling confirm method.

```typescript
import { Component, inject } from '@angular/core';
import { ButtonModule } from '@openng/optimus-ui/button';
import { ConfirmPopupModule } from '@openng/optimus-ui/confirmpopup';
import { ToastModule } from '@openng/optimus-ui/toast';
import { MessageService, ConfirmationService } from '@openng/optimus-ui/api';

@Component({
    template: `
        <div class="card flex justify-center gap-2">
            <p-toast />
            <p-confirmpopup />
            <p-button (onClick)="confirm1($event)" label="Save" [outlined]="true" />
            <p-button (onClick)="confirm2($event)" label="Delete" severity="danger" [outlined]="true" />
        </div>
    `,
    standalone: true,
    imports: [ButtonModule, ConfirmPopupModule, ToastModule],
    providers: [ConfirmationService, MessageService]
})
export class ConfirmpopupBasicDemo {
    private confirmationService = inject(ConfirmationService);
    private messageService = inject(MessageService);

    confirm2(event: Event) {
        this.confirmationService.confirm({
            target: event.currentTarget as EventTarget,
            message: 'Do you want to delete this record?',
            icon: 'pi pi-info-circle',
            rejectButtonProps: {
                label: 'Cancel',
                severity: 'secondary',
                outlined: true
            },
            acceptButtonProps: {
                label: 'Delete',
                severity: 'danger'
            },
            accept: () => {
                this.messageService.add({ severity: 'info', summary: 'Confirmed', detail: 'Record deleted', life: 3000 });
            },
            reject: () => {
                this.messageService.add({ severity: 'error', summary: 'Rejected', detail: 'You have rejected', life: 3000 });
            }
        });
    }
}
```

## Template

Content section can be customized using content template.

```typescript
import { Component, inject } from '@angular/core';
import { ButtonModule } from '@openng/optimus-ui/button';
import { ConfirmPopupModule } from '@openng/optimus-ui/confirmpopup';
import { ToastModule } from '@openng/optimus-ui/toast';
import { MessageService, ConfirmationService } from '@openng/optimus-ui/api';

@Component({
    template: `
        <div class="card flex justify-center">
            <p-toast />
            <p-confirmpopup>
                <ng-template #content let-message>
                    <div class="flex flex-col items-center w-full gap-4 border-b border-surface-200 dark:border-surface-700 p-4 mb-4 pb-0">
                        <i [class]="message.icon" class="!text-6xl text-primary-500"></i>
                        <p>{{ message.message }}</p>
                    </div>
                </ng-template>
            </p-confirmpopup>
            <p-button (click)="confirm($event)" label="Save" />
        </div>
    `,
    standalone: true,
    imports: [ButtonModule, ConfirmPopupModule, ToastModule],
    providers: [ConfirmationService, MessageService]
})
export class ConfirmpopupTemplateDemo {
    private confirmationService = inject(ConfirmationService);
    private messageService = inject(MessageService);
}
```

## Headless

Headless mode allows you to customize the entire user interface instead of the default elements.

```typescript
import { Component, inject } from '@angular/core';
import { ButtonModule } from '@openng/optimus-ui/button';
import { ConfirmPopupModule } from '@openng/optimus-ui/confirmpopup';
import { ToastModule } from '@openng/optimus-ui/toast';
import { MessageService, ConfirmationService } from '@openng/optimus-ui/api';

@Component({
    template: `
        <div class="card flex justify-center">
            <p-toast />
            <p-confirmpopup #cp>
                <ng-template #headless let-message>
                    <div class="rounded p-4">
                        <span>{{ message.message }}</span>
                        <div class="flex items-center gap-2 mt-4">
                            <p-button (onClick)="cp.onAccept()" label="Save" size="small" [autofocus]="true" />
                            <p-button (onClick)="cp.onReject()" label="Cancel" [text]="true" size="small" severity="secondary" />
                        </div>
                    </div>
                </ng-template>
            </p-confirmpopup>
            <p-button (onClick)="confirm($event)" label="Save" />
        </div>
    `,
    standalone: true,
    imports: [ButtonModule, ConfirmPopupModule, ToastModule],
    providers: [ConfirmationService, MessageService]
})
export class ConfirmpopupHeadlessDemo {
    private confirmationService = inject(ConfirmationService);
    private messageService = inject(MessageService);
}
```

## Accessibility

Screen Reader ConfirmPopup component uses alertdialog role and since any attribute is passed to the root element you may define attributes like aria-label or aria-labelledby to describe the popup contents. In addition aria-modal is added since focus is kept within the popup. It is recommended to use a trigger component that can be accessed with keyboard such as a button, if not adding tabIndex would be necessary. ConfirmPopup adds aria-expanded state attribute and aria-controls to the trigger so that the relation between the trigger and the popup is defined. Overlay Keyboard Support When the popup gets opened, the first focusable element receives the focus and this can be customized by adding autofocus to an element within the popup. Key Function tab Moves focus to the next the focusable element within the popup. shift + tab Moves focus to the previous the focusable element within the popup. escape Closes the popup and moves focus to the trigger. Buttons Keyboard Support Key Function enter Triggers the action, closes the popup and moves focus to the trigger. space Triggers the action, closes the popup and moves focus to the trigger.

## Confirmationapi

```typescript
import { Component } from '@angular/core';

@Component({
    template: `
        <div class="doc-tablewrapper">
            <table class="doc-table">
                <thead>
                    <tr>
                        <th>Name</th>
                        <th>Type</th>
                        <th>Default</th>
                        <th>Description</th>
                    </tr>
                </thead>
                <tbody>
                    <tr>
                        <td>message</td>
                        <td>string</td>
                        <td>null</td>
                        <td>Message of the confirmation.</td>
                    </tr>
                    <tr>
                        <td>key</td>
                        <td>string</td>
                        <td>null</td>
                        <td>Optional key to match the key of the confirm popup, necessary to use when component tree has multiple confirm popups.</td>
                    </tr>
                    <tr>
                        <td>icon</td>
                        <td>string</td>
                        <td>null</td>
                        <td>Icon to display next to the message.</td>
                    </tr>
                    <tr>
                        <td>accept</td>
                        <td>Function</td>
                        <td>null</td>
                        <td>Callback to execute when action is confirmed.</td>
                    </tr>
                    <tr>
                        <td>reject</td>
                        <td>Function</td>
                        <td>null</td>
                        <td>Callback to execute when action is rejected.</td>
                    </tr>
                    <tr>
                        <td>acceptLabel</td>
                        <td>string</td>
                        <td>null</td>
                        <td>Label of the accept button.</td>
                    </tr>
                    <tr>
                        <td>rejectLabel</td>
                        <td>string</td>
                        <td>null</td>
                        <td>Label of the reject button.</td>
                    </tr>
                    <tr>
                        <td>acceptIcon</td>
                        <td>string</td>
                        <td>null</td>
                        <td>Icon of the accept button.</td>
                    </tr>
                    <tr>
                        <td>rejectIcon</td>
                        <td>string</td>
                        <td>null</td>
                        <td>Icon of the reject button.</td>
                    </tr>
                    <tr>
                        <td>acceptVisible</td>
                        <td>boolean</td>
                        <td>true</td>
                        <td>Visibility of the accept button.</td>
                    </tr>
                    <tr>
                        <td>rejectVisible</td>
                        <td>boolean</td>
                        <td>true</td>
                        <td>Visibility of the reject button.</td>
                    </tr>
                    <tr>
                        <td>acceptButtonStyleClass</td>
                        <td>string</td>
                        <td>null</td>
                        <td>Style class of the accept button.</td>
                    </tr>
                    <tr>
                        <td>rejectButtonStyleClass</td>
                        <td>string</td>
                        <td>null</td>
                        <td>Style class of the reject button.</td>
                    </tr>
                    <tr>
                        <td>defaultFocus</td>
                        <td>string</td>
                        <td>accept</td>
                        <td>Element to receive the focus when the popup gets visible, valid values are "accept", "reject", and "none".</td>
                    </tr>
                </tbody>
            </table>
        </div>
    `,
    standalone: true,
    imports: []
})
export class ConfirmpopupConfirmationapiDemo {}
```

## Props

```typescript
import { Component } from '@angular/core';

@Component({
    template: `
        <div class="doc-tablewrapper">
            <table class="doc-table">
                <thead>
                    <tr>
                        <th>Name</th>
                        <th>Type</th>
                        <th>Default</th>
                        <th>Description</th>
                    </tr>
                </thead>
                <tbody>
                    <tr>
                        <td>key</td>
                        <td>string</td>
                        <td>null</td>
                        <td>Optional key to match the key of confirm object, necessary to use when component tree has multiple confirm dialogs.</td>
                    </tr>
                    <tr>
                        <td>showTransitionOptions</td>
                        <td>string</td>
                        <td>.12s cubic-bezier(0, 0, 0.2, 1)</td>
                        <td>Transition options of the show animation.</td>
                    </tr>
                    <tr>
                        <td>hideTransitionOptions</td>
                        <td>string</td>
                        <td>.1s linear</td>
                        <td>Transition options of the hide animation.</td>
                    </tr>
                    <tr>
                        <td>autoZIndex</td>
                        <td>boolean</td>
                        <td>true</td>
                        <td>Whether to automatically manage layering.</td>
                    </tr>
                    <tr>
                        <td>baseZIndex</td>
                        <td>number</td>
                        <td>0</td>
                        <td>Base zIndex value to use in layering.</td>
                    </tr>
                    <tr>
                        <td>style</td>
                        <td>string</td>
                        <td>null</td>
                        <td>Inline style of the component.</td>
                    </tr>
                    <tr>
                        <td>styleClass</td>
                        <td>string</td>
                        <td>null</td>
                        <td>Style class of the component.</td>
                    </tr>
                </tbody>
            </table>
        </div>
    `,
    standalone: true,
    imports: []
})
export class ConfirmpopupPropsDemo {}
```

## Templates

```typescript
import { Component } from '@angular/core';

@Component({
    template: `
        <div class="doc-tablewrapper">
            <table class="doc-table">
                <thead>
                    <tr>
                        <th>Name</th>
                        <th>Parameters</th>
                    </tr>
                </thead>
                <tbody>
                    <tr>
                        <td>accepticon</td>
                        <td>-</td>
                    </tr>
                    <tr>
                        <td>rejecticon</td>
                        <td>-</td>
                    </tr>
                </tbody>
            </table>
        </div>
    `,
    standalone: true,
    imports: []
})
export class ConfirmpopupTemplatesDemo {}
```

## API

### ConfirmPopup

Selector: `p-confirmpopup` · Component, standalone

ConfirmPopup displays a confirmation overlay displayed relatively to its target.

#### Inputs

| Name | Type | Default | Description |
|---|---|---|---|
| `appendTo` | `any` | `'body'` | Target element to attach the overlay, valid values are "body" or a local ng-template variable of another element (note: use binding with brackets for template variables, e.g. [appendTo]="mydiv" for a div element having #mydiv as variable name). |
| `autoZIndex` | `boolean` | `true` | Whether to automatically manage layering. |
| `baseZIndex` | `number` | `0` | Base zIndex value to use in layering. |
| `defaultFocus` | `string` | `'accept'` | Element to receive the focus when the popup gets visible, valid values are "accept", "reject", and "none". |
| `dt` | `Object \| undefined` | - | Defines scoped design tokens of the component. |
| `hideTransitionOptions` | `string` | `'.1s linear'` | Transition options of the hide animation. **Deprecated**: since v21.0.0. Use `motionOptions` instead. |
| `key` | `string \| undefined` | - | Optional key to match the key of confirm object, necessary to use when component tree has multiple confirm dialogs. |
| `motionOptions` | `MotionOptions \| undefined` | - | The motion options. |
| `pt` | `ConfirmPopupPassThrough` | - | Used to pass attributes to DOM elements inside the component. |
| `ptOptions` | `PassThroughOptions \| undefined` | - | Used to configure passthrough(pt) options of the component. |
| `showTransitionOptions` | `string` | `'.12s cubic-bezier(0, 0, 0.2, 1)'` | Transition options of the show animation. **Deprecated**: since v21.0.0. Use `motionOptions` instead. |
| `style` | `{ [klass: string]: any; } \| null \| undefined` | - | Inline style of the component. |
| `styleClass` | `string \| undefined` | - | Style class of the component. |
| `unstyled` | `boolean \| undefined` | - | Indicates whether the component should be rendered without styles. |
| `visible` | `boolean \| undefined` | - | Defines if the component is visible. |

#### Templates

Define with `<ng-template #name let-context>` inside the `p-confirmpopup` element.

| Name | Context | Description |
|---|---|---|
| `content` | `ConfirmPopupContentTemplateContext` | Custom content template. |
| `accepticon` | - | Custom accept icon template. |
| `rejecticon` | - | Custom reject icon template. |
| `headless` | `ConfirmPopupHeadlessTemplateContext` | Custom headless template. |

### Confirmation

Represents a confirmation dialog configuration.

| Name | Type | Description |
|---|---|---|
| `message?` | `string \| undefined` | The message to be displayed in the confirmation dialog. |
| `key?` | `string \| undefined` | A unique key to identify the confirmation dialog. |
| `icon?` | `string \| undefined` | The name of the icon to be displayed in the confirmation dialog. |
| `header?` | `string \| undefined` | The header text of the confirmation dialog. |
| `accept?` | `Function \| undefined` | The callback function to be executed when the accept button is clicked. |
| `reject?` | `Function \| undefined` | The callback function to be executed when the reject button is clicked. |
| `acceptLabel?` | `string \| undefined` | The label text for the accept button. |
| `rejectLabel?` | `string \| undefined` | The label text for the reject button. |
| `acceptIcon?` | `string \| undefined` | The name of the icon to be displayed on the accept button. |
| `rejectIcon?` | `string \| undefined` | The name of the icon to be displayed on the reject button. |
| `acceptVisible?` | `boolean \| undefined` | Specifies whether the accept button should be visible. |
| `rejectVisible?` | `boolean \| undefined` | Specifies whether the reject button should be visible. |
| `blockScroll?` | `boolean \| undefined` | Specifies whether to block scrolling on the page when the confirmation dialog is displayed. |
| `closeOnEscape?` | `boolean \| undefined` | Specifies whether the confirmation dialog should be closed when the escape key is pressed. |
| `dismissableMask?` | `boolean \| undefined` | Specifies whether clicking outside the confirmation dialog should dismiss it. |
| `defaultFocus?` | `string \| undefined` | The ID or class name of the element to receive focus by default when the confirmation dialog is opened. |
| `acceptButtonStyleClass?` | `string \| undefined` | The CSS class name to be applied to the accept button. |
| `rejectButtonStyleClass?` | `string \| undefined` | The CSS class name to be applied to the reject button. |
| `target?` | `EventTarget \| undefined` | The target event where the confirmation dialog is triggered from. |
| `acceptEvent?` | `EventEmitter<any> \| undefined` | An event emitter for the accept event. |
| `rejectEvent?` | `EventEmitter<any> \| undefined` | An event emitter for the reject event. |
| `acceptButtonProps?` | `any` | Accept button properties. |
| `rejectButtonProps?` | `any` | Reject button properties. |
| `closeButtonProps?` | `any` | Close button properties. |
| `closable?` | `boolean \| undefined` | Defines if the dialog is closable. |
| `position?` | `string \| undefined` | Defines the dialog position. |
| `modal?` | `boolean \| undefined` | Specifies whether the dialog displayed as modal or not. |

## Pass Through Options

| Name | Type | Description |
|---|---|---|
| `host` | `PassThroughOption<HTMLElement, I>` | Used to pass attributes to the host's DOM element. |
| `root` | `PassThroughOption<HTMLDivElement, I>` | Used to pass attributes to the root's DOM element. |
| `content` | `PassThroughOption<HTMLDivElement, I>` | Used to pass attributes to the content's DOM element. |
| `icon` | `PassThroughOption<HTMLElement, I>` | Used to pass attributes to the icon's DOM element. |
| `message` | `PassThroughOption<HTMLSpanElement, I>` | Used to pass attributes to the message's DOM element. |
| `footer` | `PassThroughOption<HTMLDivElement, I>` | Used to pass attributes to the footer's DOM element. |
| `pcRejectButton` | `ButtonPassThrough` | Used to pass attributes to the reject Button component. |
| `pcAcceptButton` | `ButtonPassThrough` | Used to pass attributes to the accept Button component. |
| `motion` | `MotionOptions` | Used to pass options to the motion component/directive. |

## Theming

### CSS Classes

| Class | Description |
|---|---|
| `p-confirmpopup` | Class name of the root element |
| `p-confirmpopup-content` | Class name of the content element |
| `p-confirmpopup-icon` | Class name of the icon element |
| `p-confirmpopup-message` | Class name of the message element |
| `p-confirmpopup-footer` | Class name of the footer element |
| `p-confirmpopup-reject-button` | Class name of the reject button element |
| `p-confirmpopup-accept-button` | Class name of the accept button element |

### Design Tokens

| Token | CSS Variable | Description |
|---|---|---|
| `confirmpopup.background` | `--p-confirmpopup-background` | Background of root |
| `confirmpopup.border.color` | `--p-confirmpopup-border-color` | Border color of root |
| `confirmpopup.color` | `--p-confirmpopup-color` | Color of root |
| `confirmpopup.border.radius` | `--p-confirmpopup-border-radius` | Border radius of root |
| `confirmpopup.shadow` | `--p-confirmpopup-shadow` | Shadow of root |
| `confirmpopup.gutter` | `--p-confirmpopup-gutter` | Gutter of root |
| `confirmpopup.arrow.offset` | `--p-confirmpopup-arrow-offset` | Arrow offset of root |
| `confirmpopup.content.padding` | `--p-confirmpopup-content-padding` | Padding of content |
| `confirmpopup.content.gap` | `--p-confirmpopup-content-gap` | Gap of content |
| `confirmpopup.icon.size` | `--p-confirmpopup-icon-size` | Size of icon |
| `confirmpopup.icon.color` | `--p-confirmpopup-icon-color` | Color of icon |
| `confirmpopup.footer.gap` | `--p-confirmpopup-footer-gap` | Gap of footer |
| `confirmpopup.footer.padding` | `--p-confirmpopup-footer-padding` | Padding of footer |
