# Dynamic Dialog

Dialogs can be created dynamically with any component as the content using a DialogService.

## Import

```typescript
import { DynamicDialogModule, DynamicDialog, DialogService } from '@openng/optimus-ui/dynamicdialog';
```

## Usage

To use dynamic dialog, a reference should be declared as DynamicDialogRef after the DialogService injected into the component.

## Opening a Dialog

The open method of the DialogService is used to open a Dialog. First parameter is the component to load and second one is the configuration object to customize the Dialog.

## Customization

DynamicDialog uses the Dialog component internally, visit dialog for more information about the available props.

## Passing Data

To pass data to a dynamically loaded component, you can use either the data or inputValues property, depending on your requirements. The data property is ideal for passing generic information that is not directly tied to the component's inputs, while inputValues allows you to set specific input properties on the component in a more structured and type-safe way. Both properties can be used together or independently, offering flexibility to meet different use cases. Additionally, the loaded component can control the dialog using the DynamicDialogRef API, providing complete control over the dialog lifecycle. Both DynamicDialogConfig and DynamicDialogRef are injectable through the constructor.

## Closing a Dialog

Most of the time, requirement is returning a value from the dialog. DialogRef's close method is used for this purpose where the parameter passed will be available at the onClose event at the caller. Here is an example on how to close the dialog from the ProductListDemo by passing a selected product.

## Example

Dynamic dialogs require an instance of a DialogService that is responsible for displaying a dialog with a component as its content. Calling open method of DialogService will display dynamic dialog. First parameter of open method is the type of component to load and the second parameter is the configuration of the Dialog such as header , width and more.

```typescript
import { Component, inject } from '@angular/core';
import { ButtonModule } from '@openng/optimus-ui/button';
import { ToastModule } from '@openng/optimus-ui/toast';
import { MessageService, DialogService } from '@openng/optimus-ui/api';
import { Product } from '@/domain/product';
import { Dialog } from '@openng/optimus-ui/dialog';

@Component({
    template: `
        <div class="card flex justify-center">
            <p-toast />
            <p-button (click)="show()" icon="pi pi-search" label="Select a Product" />
        </div>
    `,
    standalone: true,
    imports: [ButtonModule, ToastModule],
    providers: [DialogService, MessageService]
})
export class DynamicdialogExampleDemo {
    private dialogService = inject(DialogService);
    private messageService = inject(MessageService);

    show() {
        this.ref = this.dialogService.open(ProductListDemo, {
            header: 'Product List',
            modal: true,
            width: '50vw',
            closable: true,
            contentStyle: { overflow: 'auto' },
            breakpoints: {
                '960px': '75vw',
                '640px': '90vw'
            },
            templates: {
                footer: Footer
            }
        });
        
        this.ref.onClose.subscribe((data: any) => {
            let summary_and_detail;
            if (data) {
                const buttonType = data?.buttonType;
                summary_and_detail = buttonType ? { summary: 'No Product Selected', detail: `Pressed '${buttonType}' button` } : { summary: 'Product Selected', detail: data?.name };
            } else {
                summary_and_detail = { summary: 'No Product Selected', detail: 'Pressed Close button' };
            }
            this.messageService.add({ severity: 'info', ...summary_and_detail, life: 3000 });
        });
        
        this.ref.onMaximize.subscribe((value) => {
            this.messageService.add({
                severity: 'info',
                summary: 'Maximized',
                detail: `maximized: ${value.maximized}`
            });
        });
    }
}
```

## Style

Following is the list of structural style classes, for theming classes visit theming page.

## API

### DynamicDialog

Selector: `p-dynamicDialog, p-dynamicdialog, p-dynamic-dialog` · Component, standalone

#### Inputs

| Name | Type | Default | Description |
|---|---|---|---|
| `dt` | `Object \| undefined` | - | Defines scoped design tokens of the component. |
| `pt` | `DialogPassThrough` | - | Used to pass attributes to DOM elements inside the component. |
| `ptOptions` | `PassThroughOptions \| undefined` | - | Used to configure passthrough(pt) options of the component. |
| `unstyled` | `boolean \| undefined` | - | Indicates whether the component should be rendered without styles. |

### DynamicDialogRef

Dynamic Dialog instance.

| Name | Type | Description |
|---|---|---|
| `close` | `(result?: any) => void` | Closes dialog. |
| `destroy` | `() => void` | Destroys the dialog instance. |
| `dragStart` | `(event: MouseEvent) => void` | Callback to invoke on drag start. |
| `dragEnd` | `(event: MouseEvent) => void` | Callback to invoke on drag end. |
| `resizeInit` | `(event: MouseEvent) => void` | Callback to invoke on resize start. |
| `resizeEnd` | `(event: MouseEvent) => void` | Callback to invoke on resize start. |
| `maximize` | `(value: any) => void` | Callback to invoke on dialog is maximized. |
| `onClose` | `Observable<any>` | Event triggered on dialog is closed. |
| `onDestroy` | `Observable<any>` | Event triggered on dialog instance is destroyed. |
| `onDragStart` | `Observable<any>` | Event triggered on drag start. |
| `onDragEnd` | `Observable<any>` | Event triggered on drag end. |
| `onResizeInit` | `Observable<any>` | Event triggered on resize start. |
| `onResizeEnd` | `Observable<any>` | Event triggered on resize end. |
| `onMaximize` | `Observable<any>` | Event triggered on dialog is maximized. |
| `onChildComponentLoaded` | `Subject<ComponentType>` | Event triggered on child component load. |

### DynamicDialogConfig

Dialogs can be created dynamically with any component as the content using a DialogService.

| Name | Type | Description |
|---|---|---|
| `data` | `DataType \| undefined` | An object to pass to the component loaded inside the Dialog. |
| `inputValues` | `InputValuesType \| undefined` | An object to pass to the component loaded inside the Dialog. |
| `header` | `string \| undefined` | Header text of the dialog. |
| `ariaLabelledBy` | `string \| undefined` | Identifies the element (or elements) that labels the element it is applied to. |
| `footer` | `string \| undefined` | Footer text of the dialog. |
| `width` | `string \| undefined` | Width of the dialog. |
| `height` | `string \| undefined` | Height of the dialog. |
| `closeOnEscape` | `boolean \| undefined` | Specifies if pressing escape key should hide the dialog. |
| `focusOnShow` | `boolean \| undefined` | Specifies if autofocus should happen on show. |
| `focusTrap` | `boolean \| undefined` | When enabled, can only focus on elements inside the dialog. |
| `baseZIndex` | `number \| undefined` | Base zIndex value to use in layering. |
| `autoZIndex` | `boolean \| undefined` | Whether to re-enforce layering through applying zIndex. |
| `dismissableMask` | `boolean \| undefined` | Specifies if clicking the modal background should hide the dialog. |
| `rtl` | `boolean \| undefined` | Inline style of the component. |
| `style` | `{ [klass: string]: any; } \| null \| undefined` | Inline style of the component. |
| `contentStyle` | `{ [klass: string]: any; } \| null \| undefined` | Inline style of the content. |
| `styleClass` | `string \| undefined` | Style class of the component. |
| `transitionOptions` | `string \| undefined` | Transition options of the animation. |
| `closable` | `boolean \| undefined` | Adds a close icon to the header to hide the dialog. |
| `showHeader` | `boolean \| undefined` | Whether to show the header or not. |
| `modal` | `boolean \| undefined` | Defines if background should be blocked when dialog is displayed. |
| `maskStyleClass` | `string \| undefined` | Style class of the mask. |
| `resizable` | `boolean \| undefined` | Enables resizing of the content. |
| `draggable` | `boolean \| undefined` | Enables dragging to change the position using header. |
| `keepInViewport` | `boolean \| undefined` | Keeps dialog in the viewport. |
| `minX` | `number \| undefined` | Minimum value for the left coordinate of dialog in dragging. |
| `minY` | `number \| undefined` | Minimum value for the top coordinate of dialog in dragging. |
| `maximizable` | `boolean \| undefined` | Whether the dialog can be displayed full screen. |
| `maximizeIcon` | `string \| undefined` | Name of the maximize icon. |
| `minimizeIcon` | `string \| undefined` | Name of the minimize icon. |
| `position` | `string \| undefined` | Position of the dialog, options are "center", "top", "bottom", "left", "right", "topleft", "topright", "bottomleft" or "bottomright". |
| `closeAriaLabel` | `string \| undefined` | Defines a string that labels the close button for accessibility. |
| `appendTo` | `any` | Target element to attach the overlay, valid values are "body" or a local ng-template variable of another element (note: use binding with brackets for template variables, e.g. [appendTo]="mydiv" for a div element having #mydiv as variable name). |
| `duplicate` | `boolean \| undefined` | A boolean to determine if it can be duplicate. |
| `breakpoints` | `any` | Object literal to define widths per screen size. |
| `templates` | `DynamicDialogTemplates \| undefined` | Dialog templates. |
| `pt` | `DialogPassThrough` | Used to pass attributes to DOM elements inside the Dialog component. |
| `unstyled` | `boolean \| undefined` | Indicates whether the component should be rendered without styles. |

## Theming

### CSS Classes

| Class | Description |
|---|---|
| `p-dialog-mask` | Class name of the mask element |
| `p-dialog` | Class name of the root element |
| `p-dialog-header` | Class name of the header element |
| `p-dialog-title` | Class name of the title element |
| `p-dialog-header-actions` | Class name of the header actions element |
| `p-dialog-maximize-button` | Class name of the maximize button element |
| `p-dialog-close-button` | Class name of the close button element |
| `p-dialog-content` | Class name of the content element |
| `p-dialog-footer` | Class name of the footer element |
