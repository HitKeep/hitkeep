# SelectButton

SelectButton is used to choose single or multiple items from a list using buttons.

## Import

```typescript
import { SelectButtonModule, SelectButton } from '@openng/optimus-ui/selectbutton';
```

## Basic

SelectButton requires a value to bind and a collection of options.

```typescript
import { Component } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { SelectButtonModule } from '@openng/optimus-ui/selectbutton';

@Component({
    template: `
        <div class="card flex justify-center">
            <p-selectbutton [options]="stateOptions" [(ngModel)]="value" optionLabel="label" optionValue="value" aria-labelledby="basic" />
        </div>
    `,
    standalone: true,
    imports: [SelectButtonModule, FormsModule]
})
export class SelectbuttonBasicDemo {
    stateOptions: any[];
    value: string = 'one-way';
}
```

## Multiple

SelectButton allows selecting only one item by default and setting multiple option enables choosing more than one item. In multiple case, model property should be an array.

```typescript
import { Component } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { SelectButtonModule } from '@openng/optimus-ui/selectbutton';

@Component({
    template: `
        <div class="card flex justify-center">
            <p-selectbutton [options]="paymentOptions" [(ngModel)]="value" [multiple]="true" optionLabel="name" optionValue="value" />
        </div>
    `,
    standalone: true,
    imports: [SelectButtonModule, FormsModule]
})
export class SelectbuttonMultipleDemo {
    paymentOptions: any[];
    value!: number;
}
```

## Template

For custom content support define a template named item where the default local template variable refers to an option.

```typescript
import { Component } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { SelectButtonModule } from '@openng/optimus-ui/selectbutton';

@Component({
    template: `
        <div class="card flex justify-center">
            <p-selectbutton [options]="justifyOptions" [(ngModel)]="value" optionLabel="justify">
                <ng-template #item let-item>
                    <i [class]="item.icon"></i>
                </ng-template>
            </p-selectbutton>
        </div>
    `,
    standalone: true,
    imports: [SelectButtonModule, FormsModule]
})
export class SelectbuttonTemplateDemo {
    value: any;
    justifyOptions: any[];
}
```

## Sizes

SelectButton provides small and large sizes as alternatives to the base.

```typescript
import { Component } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { SelectButtonModule } from '@openng/optimus-ui/selectbutton';

@Component({
    template: `
        <div class="card flex flex-col items-center gap-4">
            <p-selectbutton [(ngModel)]="value1" [options]="options" size="small" />
            <p-selectbutton [(ngModel)]="value2" [options]="options" />
            <p-selectbutton [(ngModel)]="value3" [options]="options" size="large" />
        </div>
    `,
    standalone: true,
    imports: [SelectButtonModule, FormsModule]
})
export class SelectbuttonSizesDemo {
    value1!: string;
    value2: string = 'Beginner';
    value3: string = 'Expert';
    options: any[];
}
```

## Fluid

The fluid prop makes the component take up the full width of its container when set to true.

```typescript
import { Component } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { SelectButtonModule } from '@openng/optimus-ui/selectbutton';

@Component({
    template: `
        <div class="card">
            <p-selectbutton [options]="stateOptions" [(ngModel)]="value" optionLabel="label" optionValue="value" fluid />
        </div>
    `,
    standalone: true,
    imports: [SelectButtonModule, FormsModule]
})
export class SelectbuttonFluidDemo {
    stateOptions: any[];
    value: string = 'one-way';
}
```

## Disabled

When disabled is present, the element cannot be edited and focused entirely. Certain options can also be disabled using the optionDisabled property.

```typescript
import { Component } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { SelectButtonModule } from '@openng/optimus-ui/selectbutton';

@Component({
    template: `
        <div class="card flex justify-center flex-wrap gap-4">
            <p-selectbutton [options]="stateOptions" [(ngModel)]="value1" optionLabel="label" optionValue="value" [disabled]="true" />
            <p-selectbutton [options]="stateOptions2" [(ngModel)]="value2" optionLabel="label" optionValue="value" optionDisabled="constant" />
        </div>
    `,
    standalone: true,
    imports: [SelectButtonModule, FormsModule]
})
export class SelectbuttonDisabledDemo {
    stateOptions: any[];
    stateOptions2: any[];
    value1: string = 'off';
    value2: string = 'Option 1';
}
```

## Invalid

The invalid state is applied using the ⁠invalid property to indicate failed validation, which can be integrated with Angular Forms.

```typescript
import { Component } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { SelectButtonModule } from '@openng/optimus-ui/selectbutton';

@Component({
    template: `
        <div class="card flex justify-center">
            <p-selectbutton [options]="stateOptions" [(ngModel)]="value" optionLabel="label" optionValue="value" [invalid]="value === undefined" />
        </div>
    `,
    standalone: true,
    imports: [SelectButtonModule, FormsModule]
})
export class SelectbuttonInvalidDemo {
    stateOptions: any[];
    value: string | undefined;
}
```

## Accessibility

Screen Reader The container element that wraps the buttons has a group role whereas each button element uses button role and aria-pressed is updated depending on selection state. Value to describe an option is automatically set using the ariaLabel property that refers to the label of an option so it is still suggested to define a label even the option display consists of presentational content like icons only.

## Reactiveforms

SelectButton can also be used with reactive forms. In this case, the formControlName property is used to bind the component to a form control.

```typescript
import { Component, inject } from '@angular/core';
import { ReactiveFormsModule } from '@angular/forms';
import { MessageModule } from '@openng/optimus-ui/message';
import { SelectButtonModule } from '@openng/optimus-ui/selectbutton';
import { ToastModule } from '@openng/optimus-ui/toast';
import { ButtonModule } from '@openng/optimus-ui/button';
import { MessageService } from '@openng/optimus-ui/api';

@Component({
    template: `
        <p-toast />
        <div class="card flex justify-center">
            <form [formGroup]="exampleForm" (ngSubmit)="onSubmit()" class="flex flex-col gap-4">
                <div class="flex flex-col gap-1">
                    <p-selectbutton [options]="stateOptions" formControlName="value" [invalid]="isInvalid('value')" optionLabel="label" optionValue="value" />
                    @if (isInvalid('value')) {
                        <p-message severity="error" size="small" variant="simple">Selection is required</p-message>
                    }
                </div>
                <button pButton type="submit"><span pButtonLabel>Submit</span></button>
            </form>
        </div>
    `,
    standalone: true,
    imports: [MessageModule, SelectButtonModule, ToastModule, ButtonModule, ReactiveFormsModule]
})
export class SelectbuttonReactiveformsDemo {
    messageService = inject(MessageService);
    exampleForm: FormGroup | undefined;
    formSubmitted: boolean = false;
    stateOptions: any[];

    constructor() {
        this.exampleForm = this.fb.group({
                    value: ['', Validators.required]
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
        return control?.invalid && (control.touched || this.formSubmitted);
    }
}
```

## Templatedrivenforms

```typescript
import { Component, inject } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { MessageModule } from '@openng/optimus-ui/message';
import { SelectButtonModule } from '@openng/optimus-ui/selectbutton';
import { ToastModule } from '@openng/optimus-ui/toast';
import { ButtonModule } from '@openng/optimus-ui/button';
import { MessageService } from '@openng/optimus-ui/api';

@Component({
    template: `
        <p-toast />
        <div class="card flex justify-center">
            <form #exampleForm="ngForm" (ngSubmit)="onSubmit(exampleForm)" class="flex justify-center flex-col gap-4">
                <div class="flex flex-col gap-1">
                    <p-selectbutton #model="ngModel" [(ngModel)]="value" [options]="stateOptions" optionLabel="label" optionValue="value" [invalid]="model.invalid && (model.touched || exampleForm.submitted)" required name="value" />
                    @if (model.invalid && (model.touched || exampleForm.submitted)) {
                        <p-message severity="error" size="small" variant="simple">Selection is required.</p-message>
                    }
                </div>
                <button pButton type="submit"><span pButtonLabel>Submit</span></button>
            </form>
        </div>
    `,
    standalone: true,
    imports: [MessageModule, SelectButtonModule, ToastModule, ButtonModule, FormsModule]
})
export class SelectbuttonTemplatedrivenformsDemo {
    messageService = inject(MessageService);
    value: any;
    stateOptions: any[];

    onSubmit(form: any) {
        if (form.valid) {
            this.messageService.add({ severity: 'success', summary: 'Success', detail: 'Form Submitted', life: 3000 });
            form.resetForm();
        }
    }
}
```

## API

### SelectButton

Selector: `p-selectButton, p-selectbutton, p-select-button` · Component, standalone · works with `ngModel`, `formControl` and `formControlName`

SelectButton is used to choose single or multiple items from a list using buttons.

#### Inputs

| Name | Type | Default | Description |
|---|---|---|---|
| `allowEmpty` | `boolean` | `true` | Whether selection can not be cleared. |
| `ariaLabelledBy` | `string \| undefined` | - | Establishes relationships between the component and label(s) where its value should be one or more element IDs. |
| `autofocus` | `boolean \| undefined` | - | When present, it specifies that the component should automatically get focus on load. |
| `dataKey` | `string \| undefined` | - | A property to uniquely identify a value in options. |
| `disabled` | `boolean \| undefined` | - | When present, it specifies that the component should have disabled state style. |
| `dt` | `Object \| undefined` | - | Defines scoped design tokens of the component. |
| `fluid` | `boolean \| undefined` | - | Spans 100% width of the container when enabled. |
| `invalid` | `boolean \| undefined` | - | When present, it specifies that the component should have invalid state style. |
| `multiple` | `boolean \| undefined` | - | When specified, allows selecting multiple values. |
| `name` | `string \| undefined` | - | When present, it specifies that the name of the input. |
| `optionDisabled` | `string \| undefined` | - | Name of the disabled field of an option. |
| `optionLabel` | `string \| undefined` | - | Name of the label field of an option. |
| `options` | `any[] \| undefined` | - | An array of selectitems to display as the available options. |
| `optionValue` | `string \| undefined` | - | Name of the value field of an option. |
| `pt` | `SelectButtonPassThrough` | - | Used to pass attributes to DOM elements inside the component. |
| `ptOptions` | `PassThroughOptions \| undefined` | - | Used to configure passthrough(pt) options of the component. |
| `required` | `boolean \| undefined` | - | There must be a value (if set). |
| `size` | `"large" \| "small"` | - | Specifies the size of the component. |
| `styleClass` | `string \| undefined` | - | Style class of the component. |
| `tabindex` | `number` | `0` | Index of the element in tabbing order. |
| `unselectable` | `boolean` | - | Whether selection can be cleared. |
| `unstyled` | `boolean \| undefined` | - | Indicates whether the component should be rendered without styles. |

#### Outputs

| Name | Payload | Description |
|---|---|---|
| `onChange` | `SelectButtonChangeEvent` | Callback to invoke on selection change. |
| `onOptionClick` | `SelectButtonOptionClickEvent` | Callback to invoke on input click. |

#### Templates

Define with `<ng-template #name let-context>` inside the `p-selectButton` element.

| Name | Context | Description |
|---|---|---|
| `item` | `SelectButtonItemTemplateContext` | Custom item template. |

## Pass Through Options

| Name | Type | Description |
|---|---|---|
| `host` | `PassThroughOption<HTMLElement, I>` | Used to pass attributes to the host's DOM element. |
| `root` | `PassThroughOption<HTMLDivElement, I>` | Used to pass attributes to the root's DOM element. |
| `pcToggleButton` | `ToggleButtonPassThrough` | Used to pass attributes to the ToggleButton component. |

## Theming

### CSS Classes

| Class | Description |
|---|---|
| `p-selectbutton` | Class name of the root element |

### Design Tokens

| Token | CSS Variable | Description |
|---|---|---|
| `selectbutton.border.radius` | `--p-selectbutton-border-radius` | Border radius of root |
| `selectbutton.invalid.border.color` | `--p-selectbutton-invalid-border-color` | Invalid border color of root |
