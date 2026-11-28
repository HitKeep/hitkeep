# ColorPicker

ColorPicker is an input component to select a color.

## Import

```typescript
import { ColorPickerModule, ColorPicker } from '@openng/optimus-ui/colorpicker';
```

## Basic

ColorPicker is used as a controlled input with ngModel property.

```typescript
import { Component } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { ColorPickerModule } from '@openng/optimus-ui/colorpicker';

@Component({
    template: `
        <div class="card flex justify-center">
            <p-colorpicker [(ngModel)]="color" />
        </div>
    `,
    standalone: true,
    imports: [ColorPickerModule, FormsModule]
})
export class ColorpickerBasicDemo {
    color: string | undefined;
}
```

## Inline

ColorPicker is displayed as a popup by default, add inline property to customize this behavior.

```typescript
import { Component } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { ColorPickerModule } from '@openng/optimus-ui/colorpicker';

@Component({
    template: `
        <div class="card flex justify-center">
            <p-colorpicker [(ngModel)]="color" [inline]="true" />
        </div>
    `,
    standalone: true,
    imports: [ColorPickerModule, FormsModule]
})
export class ColorpickerInlineDemo {
    color: string | undefined;
}
```

## Format

Default color format to use in value binding is hex and other possible values can be rgb and hsb using the format property.

```typescript
import { Component } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { ColorPickerModule } from '@openng/optimus-ui/colorpicker';

@Component({
    template: `
        <div class="card flex flex-wrap gap-4">
            <div class="flex-1 flex flex-col items-center">
                <label for="cp-hex" class="font-bold block mb-2"> HEX </label>
                <p-colorpicker [(ngModel)]="color" inputId="cp-hex" class="mb-4" />
                <span>{{ color }}</span>
            </div>
            <div class="flex-1 flex flex-col items-center">
                <label for="cp-rgb" class="font-bold block mb-2"> RGB </label>
                <p-colorpicker [(ngModel)]="colorRGB" format="rgb" inputId="cp-rgb" class="mb-4" />
                <span>{{ 'r:' + colorRGB.r + ' g:' + colorRGB.g + ' b:' + colorRGB.b }}</span>
            </div>
            <div class="flex-1 flex flex-col items-center">
                <label for="cp-hsb" class="font-bold block mb-2"> HSB </label>
                <p-colorpicker [(ngModel)]="colorHSB" format="hsb" inputId="cp-hsb" class="mb-4" />
                <span>{{ 'h:' + colorHSB.h + ' s:' + colorHSB.s + ' b:' + colorHSB.b }}</span>
            </div>
        </div>
    `,
    standalone: true,
    imports: [ColorPickerModule, FormsModule]
})
export class ColorpickerFormatDemo {
    color: string = '#6466f1';
    colorRGB: any = { r: 100, g: 102, b: 241 };
    colorHSB: any = { h: 239, s: 59, b: 95 };
}
```

## Disabled

When disabled is present, the element cannot be edited and focused.

```typescript
import { Component } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { ColorPickerModule } from '@openng/optimus-ui/colorpicker';

@Component({
    template: `
        <div class="card flex justify-center">
            <p-colorpicker [(ngModel)]="color" [disabled]="true" />
        </div>
    `,
    standalone: true,
    imports: [ColorPickerModule, FormsModule]
})
export class ColorpickerDisabledDemo {
    color: string | undefined;
}
```

## Accessibility

Screen Reader Specification does not cover a color picker yet and using a semantic native color picker is not consistent across browsers so currently component is not compatible with screen readers. In the upcoming versions, text fields will be introduced below the slider section to be able to pick a color using accessible text boxes in hsl, rgba and hex formats.

## Reactiveforms

ColorPicker can also be used with reactive forms. In this case, the formControlName property is used to bind the component to a form control.

```typescript
import { Component, inject } from '@angular/core';
import { ReactiveFormsModule } from '@angular/forms';
import { ColorPickerModule } from '@openng/optimus-ui/colorpicker';
import { MessageModule } from '@openng/optimus-ui/message';
import { ToastModule } from '@openng/optimus-ui/toast';
import { ButtonModule } from '@openng/optimus-ui/button';
import { MessageService } from '@openng/optimus-ui/api';

@Component({
    template: `
        <div class="card flex justify-center">
            <p-toast />
            <form [formGroup]="exampleForm" (ngSubmit)="onSubmit()" class="flex flex-col gap-4">
                <div class="flex flex-col items-center gap-2">
                    <p-colorpicker formControlName="color" defaultColor="989898" />
                    @if (isInvalid('color')) {
                        <p-message severity="error" size="small" variant="simple">Color is required.</p-message>
                    }
                </div>
                <button pButton severity="secondary" type="submit"><span pButtonLabel>Submit</span></button>
            </form>
        </div>
    `,
    standalone: true,
    imports: [ColorPickerModule, MessageModule, ToastModule, ButtonModule, ReactiveFormsModule]
})
export class ColorpickerReactiveformsDemo {
    messageService = inject(MessageService);
    exampleForm: FormGroup;
    formSubmitted: boolean = false;

    constructor() {
        this.exampleForm = this.fb.group({
                    color: ['', Validators.required]
                });
    }

    onSubmit() {
        this.formSubmitted = true;
        if (this.exampleForm.valid) {
            this.messageService.add({ severity: 'success', summary: 'Success', detail: 'Form is submitted', life: 3000 });
            this.exampleForm.reset();
            this.formSubmitted = false;
        }
    }

    isInvalid(controlName: string) {
        const control = this.exampleForm.get(controlName);
        return control?.invalid && this.formSubmitted;
    }
}
```

## Templatedrivenforms

```typescript
import { Component, inject } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { ColorPickerModule } from '@openng/optimus-ui/colorpicker';
import { MessageModule } from '@openng/optimus-ui/message';
import { ToastModule } from '@openng/optimus-ui/toast';
import { ButtonModule } from '@openng/optimus-ui/button';
import { MessageService } from '@openng/optimus-ui/api';

@Component({
    template: `
        <div class="card flex justify-center">
            <p-toast />
            <form #exampleForm="ngForm" (ngSubmit)="onSubmit(exampleForm)" class="flex flex-col gap-4">
                <div class="flex flex-col items-center gap-2">
                    <p-colorpicker name="color" [(ngModel)]="color" #colorModel="ngModel" required defaultColor="989898" />
                    @if (colorModel.invalid && (colorModel.touched || exampleForm.submitted)) {
                        <p-message severity="error" size="small" variant="simple">Color is required.</p-message>
                    }
                </div>
                <button pButton severity="secondary" type="submit"><span pButtonLabel>Submit</span></button>
            </form>
        </div>
    `,
    standalone: true,
    imports: [ColorPickerModule, MessageModule, ToastModule, ButtonModule, FormsModule]
})
export class ColorpickerTemplatedrivenformsDemo {
    messageService = inject(MessageService);
    color: string | undefined;

    onSubmit(form: any) {
        if (form.valid) {
            this.messageService.add({ severity: 'success', summary: 'Success', detail: 'Form is submitted', life: 3000 });
            form.resetForm();
        }
    }
}
```

## API

### ColorPicker

Selector: `p-colorPicker, p-colorpicker, p-color-picker` · Component, standalone · works with `ngModel`, `formControl` and `formControlName`

ColorPicker groups a collection of contents in tabs.

#### Inputs

| Name | Type | Default | Description |
|---|---|---|---|
| `appendTo` | `any` | - | Target element to attach the overlay, valid values are "body" or a local ng-template variable of another element (note: use binding with brackets for template variables, e.g. [appendTo]="mydiv" for a div element having #mydiv as variable name). |
| `autofocus` | `boolean \| undefined` | - | When present, it specifies that the component should automatically get focus on load. |
| `autoZIndex` | `boolean` | `true` | Whether to automatically manage layering. |
| `defaultColor` | `string \| undefined` | `'ff0000'` | Default color to display initially when model value is not present. |
| `disabled` | `boolean \| undefined` | - | When present, it specifies that the component should have disabled state style. |
| `dt` | `Object \| undefined` | - | Defines scoped design tokens of the component. |
| `format` | `"hex" \| "rgb" \| "hsb"` | `'hex'` | Format to use in value binding. |
| `hideTransitionOptions` | `string` | `'.1s linear'` | Transition options of the hide animation. **Deprecated**: since v21.0.0, use `motionOptions` instead. |
| `inline` | `boolean \| undefined` | - | Whether to display as an overlay or not. |
| `inputId` | `string \| undefined` | - | Identifier of the focus input to match a label defined for the dropdown. |
| `invalid` | `boolean \| undefined` | - | When present, it specifies that the component should have invalid state style. |
| `motionOptions` | `MotionOptions \| undefined` | - | The motion options. |
| `name` | `string \| undefined` | - | When present, it specifies that the name of the input. |
| `overlayOptions` | `OverlayOptions \| undefined` | - | Whether to use overlay API feature. The properties of overlay API can be used like an object in it. |
| `pt` | `ColorPickerPassThrough` | - | Used to pass attributes to DOM elements inside the component. |
| `ptOptions` | `PassThroughOptions \| undefined` | - | Used to configure passthrough(pt) options of the component. |
| `required` | `boolean \| undefined` | - | There must be a value (if set). |
| `showTransitionOptions` | `string` | `'.12s cubic-bezier(0, 0, 0.2, 1)'` | Transition options of the show animation. **Deprecated**: since v21.0.0, use `motionOptions` instead. |
| `styleClass` | `string \| undefined` | - | Style class of the component. **Deprecated**: since v20.0.0, use `class` instead. |
| `tabindex` | `string \| undefined` | - | Index of the element in tabbing order. |
| `unstyled` | `boolean \| undefined` | - | Indicates whether the component should be rendered without styles. |

#### Outputs

| Name | Payload | Description |
|---|---|---|
| `onChange` | `ColorPickerChangeEvent` | Callback to invoke on value change. |
| `onHide` | `any` | Callback to invoke on panel is hidden. |
| `onShow` | `any` | Callback to invoke on panel is shown. |

## Pass Through Options

| Name | Type | Description |
|---|---|---|
| `host` | `PassThroughOption<HTMLElement, I>` | Used to pass attributes to the host's DOM element. |
| `root` | `PassThroughOption<HTMLDivElement, I>` | Used to pass attributes to the root's DOM element. |
| `preview` | `PassThroughOption<HTMLInputElement, I>` | Used to pass attributes to the preview input's DOM element. |
| `panel` | `PassThroughOption<HTMLDivElement, I>` | Used to pass attributes to the panel's DOM element. |
| `content` | `PassThroughOption<HTMLDivElement, I>` | Used to pass attributes to the content's DOM element. |
| `colorSelector` | `PassThroughOption<HTMLDivElement, I>` | Used to pass attributes to the color selector's DOM element. |
| `colorBackground` | `PassThroughOption<HTMLDivElement, I>` | Used to pass attributes to the color background's DOM element. |
| `colorHandle` | `PassThroughOption<HTMLDivElement, I>` | Used to pass attributes to the color handle's DOM element. |
| `hue` | `PassThroughOption<HTMLDivElement, I>` | Used to pass attributes to the hue's DOM element. |
| `hueHandle` | `PassThroughOption<HTMLDivElement, I>` | Used to pass attributes to the hue handle's DOM element. |
| `motion` | `MotionOptions` | Used to pass options to the motion component/directive. |

## Theming

### CSS Classes

| Class | Description |
|---|---|
| `p-colorpicker` | Class name of the root element |
| `p-colorpicker-preview` | Class name of the preview element |
| `p-colorpicker-panel` | Class name of the panel element |
| `p-colorpicker-color-selector` | Class name of the color selector element |
| `p-colorpicker-color-background` | Class name of the color background element |
| `p-colorpicker-color-handle` | Class name of the color handle element |
| `p-colorpicker-hue` | Class name of the hue element |
| `p-colorpicker-hue-handle` | Class name of the hue handle element |

### Design Tokens

| Token | CSS Variable | Description |
|---|---|---|
| `colorpicker.transition.duration` | `--p-colorpicker-transition-duration` | Transition duration of root |
| `colorpicker.preview.width` | `--p-colorpicker-preview-width` | Width of preview |
| `colorpicker.preview.height` | `--p-colorpicker-preview-height` | Height of preview |
| `colorpicker.preview.border.radius` | `--p-colorpicker-preview-border-radius` | Border radius of preview |
| `colorpicker.preview.focus.ring.width` | `--p-colorpicker-preview-focus-ring-width` | Focus ring width of preview |
| `colorpicker.preview.focus.ring.style` | `--p-colorpicker-preview-focus-ring-style` | Focus ring style of preview |
| `colorpicker.preview.focus.ring.color` | `--p-colorpicker-preview-focus-ring-color` | Focus ring color of preview |
| `colorpicker.preview.focus.ring.offset` | `--p-colorpicker-preview-focus-ring-offset` | Focus ring offset of preview |
| `colorpicker.preview.focus.ring.shadow` | `--p-colorpicker-preview-focus-ring-shadow` | Focus ring shadow of preview |
| `colorpicker.panel.shadow` | `--p-colorpicker-panel-shadow` | Shadow of panel |
| `colorpicker.panel.border.radius` | `--p-colorpicker-panel-border-radius` | Border radius of panel |
| `colorpicker.panel.background` | `--p-colorpicker-panel-background` | Background of panel |
| `colorpicker.panel.border.color` | `--p-colorpicker-panel-border-color` | Border color of panel |
| `colorpicker.handle.color` | `--p-colorpicker-handle-color` | Color of handle |
