# ConfirmDialog

ConfirmDialog is backed by a service utilizing Observables to display confirmation windows easily that can be shared by multiple actions on the same component.

## Import

```typescript
import { ConfirmDialogModule, ConfirmDialog } from '@openng/optimus-ui/confirmdialog';
import { ConfirmationService, Confirmation } from '@openng/optimus-ui/api';
```

## Basic

ConfirmDialog is defined using p-confirmdialog tag and an instance of ConfirmationService is required to display it bycalling confirm method.

```typescript
import { Component, inject } from '@angular/core';
import { ButtonModule } from '@openng/optimus-ui/button';
import { ConfirmDialogModule } from '@openng/optimus-ui/confirmdialog';
import { ToastModule } from '@openng/optimus-ui/toast';
import { MessageService, ConfirmationService } from '@openng/optimus-ui/api';

@Component({
    template: `
        <div class="card flex justify-center gap-2">
            <p-toast />
            <p-confirmdialog />
            <p-button (click)="confirm1($event)" label="Save" [outlined]="true" />
            <p-button (click)="confirm2($event)" label="Delete" severity="danger" [outlined]="true" />
        </div>
    `,
    standalone: true,
    imports: [ButtonModule, ConfirmDialogModule, ToastModule],
    providers: [ConfirmationService, MessageService]
})
export class ConfirmdialogBasicDemo {
    private confirmationService = inject(ConfirmationService);
    private messageService = inject(MessageService);

    confirm2(event: Event) {
        this.confirmationService.confirm({
            target: event.target as EventTarget,
            message: 'Do you want to delete this record?',
            header: 'Danger Zone',
            icon: 'pi pi-info-circle',
            rejectLabel: 'Cancel',
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
                this.messageService.add({ severity: 'info', summary: 'Confirmed', detail: 'Record deleted' });
            },
            reject: () => {
                this.messageService.add({ severity: 'error', summary: 'Rejected', detail: 'You have rejected' });
            }
        });
    }
}
```

## Position

The position property of the confirm options is used to display a Dialog at all edges and corners of the screen.

```typescript
import { Component, inject } from '@angular/core';
import { ButtonModule } from '@openng/optimus-ui/button';
import { ConfirmDialogModule } from '@openng/optimus-ui/confirmdialog';
import { ToastModule } from '@openng/optimus-ui/toast';
import { MessageService, ConfirmationService } from '@openng/optimus-ui/api';
import { Dialog } from '@openng/optimus-ui/dialog';

@Component({
    template: `
        <div class="card">
            <p-toast />
            <p-confirmdialog key="positionDialog" [position]="position" />
            <div class="flex flex-wrap justify-center gap-2 mb-4">
                <p-button (click)="confirmPosition('left')" icon="pi pi-arrow-right" label="Left" severity="secondary" styleClass="min-w-40" />
                <p-button (click)="confirmPosition('right')" icon="pi pi-arrow-left" label="Right" severity="secondary" styleClass="min-w-40" />
            </div>
            <div class="flex flex-wrap justify-center gap-2 mb-4">
                <p-button (click)="confirmPosition('topleft')" icon="pi pi-arrow-down" label="TopLeft" severity="secondary" styleClass="min-w-40" />
                <p-button (click)="confirmPosition('top')" icon="pi pi-arrow-down" label="Top" severity="secondary" styleClass="min-w-40" />
                <p-button (click)="confirmPosition('topright')" icon="pi pi-arrow-down" label="TopRight" severity="secondary" styleClass="min-w-40" />
            </div>
            <div class="flex flex-wrap justify-center gap-2">
                <p-button (click)="confirmPosition('bottomleft')" icon="pi pi-arrow-up" label="BottomLeft" severity="secondary" styleClass="min-w-40" />
                <p-button (click)="confirmPosition('bottom')" icon="pi pi-arrow-up" label="Bottom" severity="secondary" styleClass="min-w-40" />
                <p-button (click)="confirmPosition('bottomright')" icon="pi pi-arrow-up" label="BottomRight" severity="secondary" styleClass="min-w-40" />
            </div>
        </div>
    `,
    standalone: true,
    imports: [ButtonModule, ConfirmDialogModule, ToastModule],
    providers: [ConfirmationService, MessageService]
})
export class ConfirmdialogPositionDemo {
    private confirmationService = inject(ConfirmationService);
    private messageService = inject(MessageService);
}
```

## Template

Properties of the dialog are defined in two ways, message , icon , header properties can either be defined using confirm method or declaratively on p-confirmDialog ng-template by header , message , icon and footer templates. If these values are unlikely to change then declarative approach would be useful, still properties defined in a ng-template can be overridden with confirm method call. In addition, buttons at footer section can be customized by passing your own UI, important note to make confirmation work with a custom UI is defining a local ng-template variable for the dialog and assign accept()-reject() methods to your own buttons.

```typescript
import { Component, inject } from '@angular/core';
import { ButtonModule } from '@openng/optimus-ui/button';
import { ConfirmDialogModule } from '@openng/optimus-ui/confirmdialog';
import { ToastModule } from '@openng/optimus-ui/toast';
import { MessageService, ConfirmationService } from '@openng/optimus-ui/api';

@Component({
    template: `
        <div class="card flex justify-center">
            <p-toast />
            <p-confirmdialog>
                <ng-template #message let-message>
                    @if (message) {
                        <div class="flex flex-col items-center w-full gap-4 border-b border-surface-200 dark:border-surface-700">
                            <i [ngClass]="message.icon" class="!text-6xl text-primary-500"></i>
                            <p>{{ message.message }}</p>
                        </div>
                    }
                </ng-template>
            </p-confirmdialog>
            <p-button (click)="confirm()" label="Save" />
        </div>
    `,
    standalone: true,
    imports: [ButtonModule, ConfirmDialogModule, ToastModule],
    providers: [ConfirmationService, MessageService]
})
export class ConfirmdialogTemplateDemo {
    private confirmationService = inject(ConfirmationService);
    private messageService = inject(MessageService);
}
```

## Headless

Headless mode allows you to customize the entire user interface instead of the default elements.

```typescript
import { Component, inject } from '@angular/core';
import { ButtonModule } from '@openng/optimus-ui/button';
import { ConfirmDialogModule } from '@openng/optimus-ui/confirmdialog';
import { ToastModule } from '@openng/optimus-ui/toast';
import { MessageService, ConfirmationService } from '@openng/optimus-ui/api';

@Component({
    template: `
        <div class="card flex justify-center">
            <p-toast />
            <p-confirmdialog #cd>
                <ng-template #headless let-message let-onAccept="onAccept" let-onReject="onReject">
                    @if (message) {
                        <div class="flex flex-col items-center p-8 bg-surface-0 dark:bg-surface-900 rounded">
                            <div class="rounded-full bg-primary text-primary-contrast inline-flex justify-center items-center h-24 w-24 -mt-20">
                                <i class="pi pi-question !text-5xl"></i>
                            </div>
                            <span class="font-bold text-2xl block mb-2 mt-6">{{ message.header }}</span>
                            <p class="mb-0">{{ message.message }}</p>
                            <div class="flex items-center gap-2 mt-6">
                                <p-button label="Save" (onClick)="onAccept()" styleClass="w-32"></p-button>
                                <p-button label="Cancel" [outlined]="true" (onClick)="onReject()" styleClass="w-32"></p-button>
                            </div>
                        </div>
                    }
                </ng-template>
            </p-confirmdialog>
            <p-button (click)="confirm()" label="Save" />
        </div>
    `,
    standalone: true,
    imports: [ButtonModule, ConfirmDialogModule, ToastModule],
    providers: [ConfirmationService, MessageService]
})
export class ConfirmdialogHeadlessDemo {
    private confirmationService = inject(ConfirmationService);
    private messageService = inject(MessageService);
}
```

## Accessibility

Screen Reader ConfirmDialog component uses alertdialog role along with aria-labelledby referring to the header element however any attribute is passed to the root element so you may use aria-labelledby to override this default behavior. In addition aria-modal is added since focus is kept within the popup. It is recommended to use a trigger component that can be accessed with keyboard such as a button, if not adding tabIndex would be necessary. When confirm function is used and a trigger is passed as a parameter, ConfirmDialog adds aria-expanded state attribute and aria-controls to the trigger so that the relation between the trigger and the popup is defined. If the dialog is controlled with the visible property aria-expanded and aria-controls need to be handled explicitly. Overlay Keyboard Support Key Function tab Moves focus to the next the focusable element within the popup. shift + tab Moves focus to the previous the focusable element within the popup. escape Closes the popup and moves focus to the trigger. Buttons Keyboard Support Key Function enter Triggers the action, closes the popup and moves focus to the trigger. space Triggers the action, closes the popup and moves focus to the trigger.

## API

### ConfirmDialog

Selector: `p-confirmDialog, p-confirmdialog, p-confirm-dialog` · Component, standalone

ConfirmDialog uses a Dialog UI that is integrated with the Confirmation API.

#### Inputs

| Name | Type | Default | Description |
|---|---|---|---|
| `acceptAriaLabel` | `string \| undefined` | - | Defines a string that labels the accept button for accessibility. |
| `acceptButtonStyleClass` | `string \| undefined` | - | Style class of the accept button. |
| `acceptIcon` | `string \| undefined` | - | Icon of the accept button. |
| `acceptLabel` | `string \| undefined` | - | Label of the accept button. |
| `acceptVisible` | `boolean` | `true` | Visibility of the accept button. |
| `appendTo` | `any` | `'body'` | Target element to attach the overlay, valid values are "body" or a local ng-template variable of another element (note: use binding with brackets for template variables, e.g. [appendTo]="mydiv" for a div element having #mydiv as variable name). |
| `autoZIndex` | `boolean` | `true` | Whether to automatically manage layering. |
| `baseZIndex` | `number` | `0` | Base zIndex value to use in layering. |
| `blockScroll` | `boolean` | `true` | Determines whether scrolling behavior should be blocked within the component. |
| `breakpoints` | `any` | - | Object literal to define widths per screen size. |
| `closable` | `boolean` | `true` | Adds a close icon to the header to hide the dialog. |
| `closeAriaLabel` | `string \| undefined` | - | Defines a string that labels the close button for accessibility. |
| `closeOnEscape` | `boolean` | `true` | Specifies if pressing escape key should hide the dialog. |
| `defaultFocus` | `"none" \| "accept" \| "reject" \| "close"` | `'accept'` | Element to receive the focus when the dialog gets visible. |
| `dismissableMask` | `boolean \| undefined` | - | Specifies if clicking the modal background should hide the dialog. |
| `draggable` | `boolean` | `true` | Enables dragging to change the position using header. |
| `dt` | `Object \| undefined` | - | Defines scoped design tokens of the component. |
| `focusTrap` | `boolean` | `true` | When enabled, can only focus on elements inside the confirm dialog. |
| `header` | `string \| undefined` | - | Title text of the dialog. |
| `icon` | `string \| undefined` | - | Icon to display next to message. |
| `key` | `string \| undefined` | - | Optional key to match the key of confirm object, necessary to use when component tree has multiple confirm dialogs. |
| `maskStyleClass` | `string \| undefined` | - | Specify the CSS class(es) for styling the mask element |
| `message` | `string \| undefined` | - | Message of the confirmation. |
| `modal` | `boolean` | `true` | Defines if background should be blocked when dialog is displayed. |
| `position` | `"center" \| "top" \| "bottom" \| "left" \| "right" \| "topleft" \| "topright" \| "bottomleft" \| "bottomright"` | `'center'` | Allows getting the position of the component. |
| `pt` | `ConfirmDialogPassThrough` | - | Used to pass attributes to DOM elements inside the component. |
| `ptOptions` | `PassThroughOptions \| undefined` | - | Used to configure passthrough(pt) options of the component. |
| `rejectAriaLabel` | `string \| undefined` | - | Defines a string that labels the reject button for accessibility. |
| `rejectButtonStyleClass` | `string \| undefined` | - | Style class of the reject button. |
| `rejectIcon` | `string \| undefined` | - | Icon of the reject button. |
| `rejectLabel` | `string \| undefined` | - | Label of the reject button. |
| `rejectVisible` | `boolean` | `true` | Visibility of the reject button. |
| `rtl` | `boolean` | `false` | When enabled dialog is displayed in RTL direction. |
| `style` | `{ [klass: string]: any; } \| null \| undefined` | - | Inline style of the element. |
| `styleClass` | `string \| undefined` | - | Class of the element. |
| `transitionOptions` | `string` | `'150ms cubic-bezier(0, 0, 0.2, 1)'` | Transition options of the animation. |
| `unstyled` | `boolean \| undefined` | - | Indicates whether the component should be rendered without styles. |
| `visible` | `any` | - | Current visible state as a boolean. |

#### Outputs

| Name | Payload | Description |
|---|---|---|
| `onHide` | `ConfirmEventType` | Callback to invoke when dialog is hidden. |

#### Templates

Define with `<ng-template #name let-context>` inside the `p-confirmDialog` element.

| Name | Context | Description |
|---|---|---|
| `header` | - | Custom header template. |
| `footer` | - | Custom footer template. |
| `rejecticon` | - | Custom reject icon template. |
| `accepticon` | - | Custom accept icon template. |
| `message` | `ConfirmDialogMessageTemplateContext` | Custom message template. |
| `icon` | - | Custom icon template. |
| `headless` | `ConfirmDialogHeadlessTemplateContext` | Custom headless template. |

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
| `root` | `DialogPassThrough` | Used to pass attributes to the root's DOM element. |
| `mask` | `PassThroughOption<HTMLDivElement, I>` | Used to pass attributes to the mask's DOM element. |
| `icon` | `PassThroughOption<HTMLElement, I>` | Used to pass attributes to the icon's DOM element. |
| `message` | `PassThroughOption<HTMLSpanElement, I>` | Used to pass attributes to the message's DOM element. |
| `resizeHandle` | `PassThroughOption<HTMLDivElement, I>` | Used to pass attributes to the resize handle's DOM element. |
| `header` | `PassThroughOption<HTMLDivElement, I>` | Used to pass attributes to the header's DOM element. |
| `title` | `PassThroughOption<HTMLSpanElement, I>` | Used to pass attributes to the title's DOM element. |
| `headerActions` | `PassThroughOption<HTMLDivElement, I>` | Used to pass attributes to the header actions' DOM element. |
| `pcCloseButton` | `ButtonPassThrough` | Used to pass attributes to the close Button component. |
| `content` | `PassThroughOption<HTMLDivElement, I>` | Used to pass attributes to the content's DOM element. |
| `footer` | `PassThroughOption<HTMLDivElement, I>` | Used to pass attributes to the footer's DOM element. |
| `pcAcceptButton` | `ButtonPassThrough` | Used to pass attributes to the accept Button component. |
| `pcRejectButton` | `ButtonPassThrough` | Used to pass attributes to the reject Button component. |

## Theming

### CSS Classes

| Class | Description |
|---|---|
| `p-confirmdialog` | Class name of the root element |
| `p-confirmdialog-icon` | Class name of the icon element |
| `p-confirmdialog-message` | Class name of the message element |
| `p-confirmdialog-reject-button` | Class name of the reject button element |
| `p-confirmdialog-accept-button` | Class name of the accept button element |

### Design Tokens

| Token | CSS Variable | Description |
|---|---|---|
| `confirmdialog.icon.size` | `--p-confirmdialog-icon-size` | Size of icon |
| `confirmdialog.icon.color` | `--p-confirmdialog-icon-color` | Color of icon |
| `confirmdialog.content.gap` | `--p-confirmdialog-content-gap` | Gap of content |
