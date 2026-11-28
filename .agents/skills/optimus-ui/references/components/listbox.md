# Listbox

Listbox is used to select one or more values from a list of items.

## Import

```typescript
import { ListboxModule, Listbox } from '@openng/optimus-ui/listbox';
```

## Basic

Listbox is used as a controlled component with ngModel property along with an options collection. Label and value of an option are defined with the optionLabel and optionValue properties respectively. Default property name for the optionLabel is label and value for the optionValue . If optionValue is omitted and the object has no value property, the object itself becomes the value of an option. Note that, when options are simple primitive values such as a string array, no optionLabel and optionValue would be necessary.

```typescript
import { Component, OnInit } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { ListboxModule } from '@openng/optimus-ui/listbox';

interface City {
    name: string;
    code: string;
}

@Component({
    template: `
        <div class="card flex justify-center">
            <p-listbox [options]="cities" [(ngModel)]="selectedCity" optionLabel="name" class="w-full md:w-56" />
        </div>
    `,
    standalone: true,
    imports: [ListboxModule, FormsModule]
})
export class ListboxBasicDemo implements OnInit {
    cities!: City[];
    selectedCity!: City;

    ngOnInit() {
        this.cities = [
            { name: 'New York', code: 'NY' },
            { name: 'Rome', code: 'RM' },
            { name: 'London', code: 'LDN' },
            { name: 'Istanbul', code: 'IST' },
            { name: 'Paris', code: 'PRS' }
        ];
    }
}
```

## Checkmark

An alternative way to highlight the selected option is displaying a checkmark instead.

```typescript
import { Component, OnInit } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { ListboxModule } from '@openng/optimus-ui/listbox';

interface City {
    name: string;
    code: string;
}

@Component({
    template: `
        <div class="card flex justify-center">
            <p-listbox [(ngModel)]="selectedCity" [options]="cities" optionLabel="name" [checkmark]="true" [highlightOnSelect]="false" class="w-full md:w-56" />
        </div>
    `,
    standalone: true,
    imports: [ListboxModule, FormsModule]
})
export class ListboxCheckmarkDemo implements OnInit {
    cities!: City[];
    selectedCity!: City;

    ngOnInit() {
        this.cities = [
            { name: 'New York', code: 'NY' },
            { name: 'Rome', code: 'RM' },
            { name: 'London', code: 'LDN' },
            { name: 'Istanbul', code: 'IST' },
            { name: 'Paris', code: 'PRS' }
        ];
    }
}
```

## Checkbox

Listbox allows item selection using checkboxes.

```typescript
import { Component, OnInit } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { ListboxModule } from '@openng/optimus-ui/listbox';

interface City {
    name: string;
    code: string;
}

@Component({
    template: `
        <div class="card flex justify-center">
            <p-listbox [(ngModel)]="selectedCity" [options]="cities" [multiple]="true" [checkbox]="true" optionLabel="name" class="w-full md:w-56" />
        </div>
    `,
    standalone: true,
    imports: [ListboxModule, FormsModule]
})
export class ListboxCheckboxDemo implements OnInit {
    cities!: City[];
    selectedCity!: City;

    ngOnInit() {
        this.cities = [
            { name: 'New York', code: 'NY' },
            { name: 'Rome', code: 'RM' },
            { name: 'London', code: 'LDN' },
            { name: 'Istanbul', code: 'IST' },
            { name: 'Paris', code: 'PRS' }
        ];
    }
}
```

## Multiple

ListBox allows choosing a single item by default, enable multiple property to choose more than one. When the optional metaKeySelection is present, behavior is changed in a way that selecting a new item requires meta key to be present.

```typescript
import { Component, OnInit } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { ListboxModule } from '@openng/optimus-ui/listbox';

interface City {
    name: string;
    code: string;
}

@Component({
    template: `
        <div class="card flex justify-center">
            <p-listbox [options]="cities" [(ngModel)]="selectedCities" optionLabel="name" [multiple]="true" [metaKeySelection]="false" class="w-full md:w-56" />
        </div>
    `,
    standalone: true,
    imports: [ListboxModule, FormsModule]
})
export class ListboxMultipleDemo implements OnInit {
    cities!: City[];
    selectedCities!: City[];

    ngOnInit() {
        this.cities = [
            { name: 'New York', code: 'NY' },
            { name: 'Rome', code: 'RM' },
            { name: 'London', code: 'LDN' },
            { name: 'Istanbul', code: 'IST' },
            { name: 'Paris', code: 'PRS' }
        ];
    }
}
```

## Group

Options can be grouped when a nested data structures is provided.

```typescript
import { Component } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { ListboxModule } from '@openng/optimus-ui/listbox';
import { SelectItemGroup } from '@openng/optimus-ui/api';
import { Country } from '@/domain/customer';

interface Country {
    name: string;
    code: string;
}

@Component({
    template: `
        <div class="card flex justify-center">
            <p-listbox [options]="groupedCities" [group]="true" [(ngModel)]="selectedCountry" class="w-full md:w-56">
                <ng-template let-group #group>
                    <div class="flex items-center">
                        <img src="https://optimus.openng.org/demo/flag/flag_placeholder.png" [class]="'mr-2 flag flag-' + group.value" style="width: 20px" />
                        <span>{{ group.label }}</span>
                    </div>
                </ng-template>
            </p-listbox>
        </div>
    `,
    standalone: true,
    imports: [ListboxModule, FormsModule]
})
export class ListboxGroupDemo {
    groupedCities!: SelectItemGroup[];
    selectedCountry!: Country;

    constructor() {
        this.groupedCities = [
                    {
                        label: 'Germany',
                        value: 'de',
                        items: [
                            { label: 'Berlin', value: 'Berlin' },
                            { label: 'Frankfurt', value: 'Frankfurt' },
                            { label: 'Hamburg', value: 'Hamburg' },
                            { label: 'Munich', value: 'Munich' }
                        ]
                    },
                    {
                        label: 'USA',
                        value: 'us',
                        items: [
                            { label: 'Chicago', value: 'Chicago' },
                            { label: 'Los Angeles', value: 'Los Angeles' },
                            { label: 'New York', value: 'New York' },
                            { label: 'San Francisco', value: 'San Francisco' }
                        ]
                    },
                    {
                        label: 'Japan',
                        value: 'jp',
                        items: [
                            { label: 'Kyoto', value: 'Kyoto' },
                            { label: 'Osaka', value: 'Osaka' },
                            { label: 'Tokyo', value: 'Tokyo' },
                            { label: 'Yokohama', value: 'Yokohama' }
                        ]
                    }
                ];
    }
}
```

## Filter

ListBox provides built-in filtering that is enabled by adding the filter property.

```typescript
import { Component, OnInit } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { ListboxModule } from '@openng/optimus-ui/listbox';

interface City {
    name: string;
    code: string;
}

@Component({
    template: `
        <div class="card flex justify-center">
            <p-listbox [options]="cities" [(ngModel)]="selectedCity" optionLabel="name" [filter]="true" class="w-full md:w-56" />
        </div>
    `,
    standalone: true,
    imports: [ListboxModule, FormsModule]
})
export class ListboxFilterDemo implements OnInit {
    cities!: City[];
    selectedCity!: City;

    ngOnInit() {
        this.cities = [
            { name: 'New York', code: 'NY' },
            { name: 'Rome', code: 'RM' },
            { name: 'London', code: 'LDN' },
            { name: 'Istanbul', code: 'IST' },
            { name: 'Paris', code: 'PRS' }
        ];
    }
}
```

## Template

For custom content support define a template named item where the default local template variable refers to an option.

```typescript
import { Component, OnInit } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { ListboxModule } from '@openng/optimus-ui/listbox';
import { Country } from '@/domain/customer';

interface Country {
    name: string;
    code: string;
}

@Component({
    template: `
        <div class="card flex justify-center">
            <p-listbox [options]="countries" [(ngModel)]="selectedCountry" optionLabel="name" class="w-full md:w-56">
                <ng-template #item let-country>
                    <div class="flex items-center gap-2">
                        <img src="https://optimus.openng.org/demo/flag/flag_placeholder.png" [class]="'flag flag-' + country.code.toLowerCase()" style="width: 18px" />
                        <div>{{ country.name }}</div>
                    </div>
                </ng-template>
            </p-listbox>
        </div>
    `,
    standalone: true,
    imports: [ListboxModule, FormsModule]
})
export class ListboxTemplateDemo implements OnInit {
    countries!: Country[];
    selectedCountry!: Country;

    ngOnInit() {
        this.countries = [
            { name: 'Australia', code: 'AU' },
            { name: 'Brazil', code: 'BR' },
            { name: 'China', code: 'CN' },
            { name: 'Egypt', code: 'EG' },
            { name: 'France', code: 'FR' },
            { name: 'Germany', code: 'DE' },
            { name: 'India', code: 'IN' },
            { name: 'Japan', code: 'JP' },
            { name: 'Spain', code: 'ES' },
            { name: 'United States', code: 'US' }
        ];
    }
}
```

## Virtual Scroll

VirtualScrolling is an efficient way of rendering the options by displaying a small subset of data in the viewport at any time. When dealing with huge number of options, it is suggested to enable VirtualScrolling to avoid performance issues. Usage is simple as setting virtualScroll property to true and defining virtualScrollItemSize to specify the height of an item.

```typescript
import { Component } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { ListboxModule } from '@openng/optimus-ui/listbox';

@Component({
    template: `
        <div class="card flex justify-center">
            <p-listbox
                [options]="items"
                [(ngModel)]="selectedItems"
                [selectAll]="selectAll"
                [filter]="true"
                [checkbox]="true"
                optionLabel="label"
                [virtualScroll]="true"
                [virtualScrollItemSize]="40"
                [multiple]="true"
                [metaKeySelection]="false"
                (onSelectAllChange)="onSelectAllChange($event)"
                (onChange)="onChange($event)"
                scrollHeight="250px"
                [striped]="true"
                class="w-full md:w-56"
            />
        </div>
    `,
    standalone: true,
    imports: [ListboxModule, FormsModule]
})
export class ListboxVirtualscrollDemo {
    items: any = Array.from({ length: 100000 }, (_, i) => ({ label: `Item #${i}`, value: i }));
    selectedItems!: any[];
    selectAll: boolean = false;

    onSelectAllChange(event) {
        this.selectedItems = event.checked ? [...this.items] : [];
        this.selectAll = event.checked;
    }

    onChange(event) {
        const { value } = event;
        if (value) this.selectAll = value.length === this.items.length;
    }
}
```

## Invalid

The invalid state is applied using the ⁠invalid property to indicate failed validation, which can be integrated with Angular Forms.

```typescript
import { Component, OnInit } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { ListboxModule } from '@openng/optimus-ui/listbox';

interface City {
    name: string;
    code: string;
}

@Component({
    template: `
        <div class="card flex justify-center">
            <p-listbox [options]="cities" [(ngModel)]="selectedCity" optionLabel="name" class="w-full md:w-56" [invalid]="selectedCity === undefined" />
        </div>
    `,
    standalone: true,
    imports: [ListboxModule, FormsModule]
})
export class ListboxInvalidDemo implements OnInit {
    cities!: City[];
    selectedCity!: City;

    ngOnInit() {
        this.cities = [
            { name: 'New York', code: 'NY' },
            { name: 'Rome', code: 'RM' },
            { name: 'London', code: 'LDN' },
            { name: 'Istanbul', code: 'IST' },
            { name: 'Paris', code: 'PRS' }
        ];
    }
}
```

## Disabled

When disabled is present, the element cannot be edited and focused.

```typescript
import { Component, OnInit } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { ListboxModule } from '@openng/optimus-ui/listbox';

interface City {
    name: string;
    code: string;
}

@Component({
    template: `
        <div class="card flex justify-center">
            <p-listbox [options]="cities" [(ngModel)]="selectedCity" optionLabel="name" [disabled]="true" class="w-full md:w-56" />
        </div>
    `,
    standalone: true,
    imports: [ListboxModule, FormsModule]
})
export class ListboxDisabledDemo implements OnInit {
    cities!: City[];
    selectedCity!: City;

    ngOnInit() {
        this.cities = [
            { name: 'New York', code: 'NY' },
            { name: 'Rome', code: 'RM' },
            { name: 'London', code: 'LDN' },
            { name: 'Istanbul', code: 'IST' },
            { name: 'Paris', code: 'PRS' }
        ];
    }
}
```

## Accessibility

Screen Reader Value to describe the component can be provided ariaLabelledBy or ariaLabel props. The list element has a listbox role with the aria-multiselectable attribute that sets to true when multiple selection is enabled. Each list item has an option role with aria-selected and aria-disabled as their attributes.

## Dragdrop

Items can be reordered using drag and drop by enabling dragdrop property. Depends on &#64;angular/cdk package.

```typescript
import { Component, OnInit } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { ListboxModule } from '@openng/optimus-ui/listbox';

@Component({
    template: `
        <div class="card flex justify-center">
            <p-listbox [options]="cities" [(ngModel)]="selectedCity" optionLabel="name" [dragdrop]="true" class="w-full md:w-56" />
        </div>
    `,
    standalone: true,
    imports: [ListboxModule, FormsModule]
})
export class ListboxDragdropDemo implements OnInit {
    cities!: any[];
    selectedCity!: any;

    ngOnInit() {
        this.cities = [
            { name: 'New York', code: 'NY' },
            { name: 'Rome', code: 'RM' },
            { name: 'London', code: 'LDN' },
            { name: 'Istanbul', code: 'IST' },
            { name: 'Paris', code: 'PRS' }
        ];
    }
}
```

## Reactiveforms

Listbox can also be used with reactive forms. In this case, the formControlName property is used to bind the component to a form control.

```typescript
import { Component, inject } from '@angular/core';
import { ReactiveFormsModule } from '@angular/forms';
import { ListboxModule } from '@openng/optimus-ui/listbox';
import { MessageModule } from '@openng/optimus-ui/message';
import { ToastModule } from '@openng/optimus-ui/toast';
import { ButtonModule } from '@openng/optimus-ui/button';
import { MessageService } from '@openng/optimus-ui/api';

interface City {
    name: string;
    code: string;
}

@Component({
    template: `
        <p-toast />
        <div class="card flex justify-center">
            <form [formGroup]="exampleForm" (ngSubmit)="onSubmit()" class="flex flex-col gap-4 sm:w-56">
                <div class="flex flex-col gap-1">
                    <p-listbox [options]="cities" formControlName="selectedCity" optionLabel="name" class="w-full md:w-56" [invalid]="isInvalid('selectedCity')" />
                    @if (isInvalid('selectedCity')) {
                        <p-message severity="error" size="small" variant="simple">City is required.</p-message>
                    }
                </div>
                <button pButton severity="secondary" type="submit"><span pButtonLabel>Submit</span></button>
            </form>
        </div>
    `,
    standalone: true,
    imports: [ListboxModule, MessageModule, ToastModule, ButtonModule, ReactiveFormsModule]
})
export class ListboxReactiveformsDemo {
    messageService = inject(MessageService);
    exampleForm: FormGroup | undefined;
    formSubmitted: boolean = false;
    cities!: City[];

    constructor() {
        this.cities = [
                    { name: 'New York', code: 'NY' },
                    { name: 'Rome', code: 'RM' },
                    { name: 'London', code: 'LDN' },
                    { name: 'Istanbul', code: 'IST' },
                    { name: 'Paris', code: 'PRS' }
                ];
                this.exampleForm = this.fb.group({
                    selectedCity: ['', Validators.required]
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
import { Component, OnInit, inject } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { ListboxModule } from '@openng/optimus-ui/listbox';
import { MessageModule } from '@openng/optimus-ui/message';
import { ToastModule } from '@openng/optimus-ui/toast';
import { ButtonModule } from '@openng/optimus-ui/button';
import { MessageService } from '@openng/optimus-ui/api';

interface City {
    name: string;
    code: string;
}

@Component({
    template: `
        <p-toast />
        <div class="card flex justify-center">
            <form #exampleForm="ngForm" (ngSubmit)="onSubmit(exampleForm)" class="flex justify-center flex-col gap-4 md:w-56">
                <div class="flex flex-col gap-1">
                    <p-listbox #city="ngModel" [options]="cities" [(ngModel)]="selectedCity" optionLabel="name" class="w-full md:w-56" [invalid]="city.invalid && exampleForm.submitted" name="city" required />
                    @if (city.invalid && exampleForm.submitted) {
                        <p-message severity="error" size="small" variant="simple">City is required.</p-message>
                    }
                </div>
                <button pButton severity="secondary" type="submit"><span pButtonLabel>Submit</span></button>
            </form>
        </div>
    `,
    standalone: true,
    imports: [ListboxModule, MessageModule, ToastModule, ButtonModule, FormsModule]
})
export class ListboxTemplatedrivenformsDemo implements OnInit {
    messageService = inject(MessageService);
    selectedCity!: City;
    cities!: City[];

    ngOnInit() {
        this.cities = [
            { name: 'New York', code: 'NY' },
            { name: 'Rome', code: 'RM' },
            { name: 'London', code: 'LDN' },
            { name: 'Istanbul', code: 'IST' },
            { name: 'Paris', code: 'PRS' }
        ];
    }

    onSubmit(form: any) {
        if (form.valid) {
            this.messageService.add({ severity: 'success', summary: 'Success', detail: 'Form Submitted', life: 3000 });
            form.resetForm();
        }
    }
}
```

## API

### Listbox

Selector: `p-listbox, p-listBox, p-list-box` · Component, standalone · works with `ngModel`, `formControl` and `formControlName`

ListBox is used to select one or more values from a list of items.

#### Inputs

| Name | Type | Default | Description |
|---|---|---|---|
| `ariaFilterLabel` | `string \| undefined` | - | Defines a string that labels the filter input. |
| `ariaLabel` | `string \| undefined` | - | Defines a string that labels the input for accessibility. |
| `autoOptionFocus` | `boolean \| undefined` | `true` | Whether to focus on the first visible or selected element when the overlay panel is shown. |
| `checkbox` | `boolean` | `false` | When specified, allows selecting items with checkboxes. |
| `checkmark` | `boolean` | `false` | Whether the selected option will be shown with a check mark. |
| `dataKey` | `string \| undefined` | - | A property to uniquely identify a value in options. |
| `disabled` | `boolean \| undefined` | - | When present, it specifies that the component should have disabled state style. |
| `dragdrop` | `boolean` | `false` | Whether to enable dragdrop based reordering. |
| `dropListData` | `any[] \| undefined` | - | Array to use for CDK drop list data binding. When not provided, uses options array. |
| `dt` | `Object \| undefined` | - | Defines scoped design tokens of the component. |
| `emptyFilterMessage` | `string \| undefined` | - | Text to display when filtering does not return any results. |
| `emptyMessage` | `string \| undefined` | - | Text to display when there is no data. Defaults to global value in i18n translation configuration. |
| `emptySelectionMessage` | `string \| undefined` | `'No selected item'` | Text to display when filtering does not return any results. Defaults to global value in i18n translation configuration. |
| `filter` | `boolean` | `false` | When specified, displays a filter input at header. |
| `filterBy` | `string \| undefined` | - | When filtering is enabled, filterBy decides which field or fields (comma separated) to search against. |
| `filterFields` | `any[] \| undefined` | - | Fields used when filtering the options, defaults to optionLabel. |
| `filterLocale` | `string \| undefined` | - | Locale to use in filtering. The default locale is the host environment's current locale. |
| `filterMatchMode` | `string` | `'contains'` | Defines how the items are filtered. |
| `filterMessage` | `string \| undefined` | - | Text to display when filtering. |
| `filterPlaceHolder` | `string \| undefined` | - | Defines placeholder of the filter input. |
| `filterValue` | `string` | - | When specified, filter displays with this value. |
| `fluid` | `boolean \| undefined` | - | Spans 100% width of the container when enabled. |
| `focusOnHover` | `boolean \| undefined` | `true` | When enabled, the hovered option will be focused. |
| `group` | `boolean \| undefined` | - | Whether to display options as grouped when nested options are provided. |
| `highlightOnSelect` | `boolean` | `true` | Whether the selected option will be add highlight class. |
| `hostName` | `any` | `''` | - |
| `id` | `string \| undefined` | - | Unique identifier of the component. |
| `invalid` | `boolean \| undefined` | - | When present, it specifies that the component should have invalid state style. |
| `lazy` | `boolean` | `false` | Defines if data is loaded and interacted with in lazy manner. |
| `listStyle` | `{ [klass: string]: any; } \| null \| undefined` | - | Inline style of the list element. |
| `listStyleClass` | `string \| undefined` | - | Style class of the list element. |
| `metaKeySelection` | `boolean` | `false` | Defines how multiple items can be selected, when true metaKey needs to be pressed to select or unselect an item and when set to false selection of each item can be toggled individually. On touch enabled devices, metaKeySelection is turned off automatically. |
| `multiple` | `boolean \| undefined` | - | When specified, allows selecting multiple values. |
| `name` | `string \| undefined` | - | When present, it specifies that the name of the input. |
| `optionDisabled` | `string \| ((item: any) => boolean) \| undefined` | - | Name of the disabled field of an option or function to determine disabled state. |
| `optionGroupChildren` | `string \| undefined` | `'items'` | Name of the options field of an option group. |
| `optionGroupLabel` | `string \| undefined` | `'label'` | Name of the label field of an option group. |
| `optionLabel` | `string \| undefined` | - | Name of the label field of an option. |
| `options` | `any[]` | - | An array of selectitems to display as the available options. |
| `optionValue` | `string \| undefined` | - | Name of the value field of an option. |
| `pt` | `ListBoxPassThrough` | - | Used to pass attributes to DOM elements inside the component. |
| `ptOptions` | `PassThroughOptions \| undefined` | - | Used to configure passthrough(pt) options of the component. |
| `readonly` | `boolean \| undefined` | - | When present, it specifies that the element value cannot be changed. |
| `required` | `boolean \| undefined` | - | There must be a value (if set). |
| `scrollHeight` | `string` | `'14rem'` | Height of the viewport in pixels, a scrollbar is defined if height of list exceeds this value. |
| `searchLocale` | `boolean \| undefined` | - | Locale to use in searching. The default locale is the host environment's current locale. |
| `searchMessage` | `string \| undefined` | `'{0} results are available'` | Text to display when the search is active. Defaults to global value in i18n translation configuration. |
| `selectAll` | `boolean \| null \| undefined` | - | Whether all data is selected. |
| `selectionMessage` | `string \| undefined` | `'{0} items selected'` | Text to be displayed in hidden accessible field when options are selected. Defaults to global value in i18n translation configuration. |
| `selectOnFocus` | `boolean \| undefined` | - | When enabled, the focused option is selected. |
| `showToggleAll` | `boolean` | `true` | Whether header checkbox is shown in multiple mode. |
| `striped` | `boolean \| undefined` | `false` | Whether to displays rows with alternating colors. |
| `styleClass` | `string \| undefined` | - | Style class of the container. **Deprecated**: since v20.0.0, use `class` instead. |
| `tabindex` | `number \| undefined` | `0` | Index of the element in tabbing order. |
| `unstyled` | `boolean \| undefined` | - | Indicates whether the component should be rendered without styles. |
| `virtualScroll` | `boolean \| undefined` | - | Whether the data should be loaded on demand during scroll. |
| `virtualScrollItemSize` | `number \| undefined` | - | Height of an item in the list for VirtualScrolling. |
| `virtualScrollOptions` | `ScrollerOptions \| undefined` | - | Whether to use the scroller feature. The properties of scroller component can be used like an object in it. |

#### Outputs

| Name | Payload | Description |
|---|---|---|
| `onBlur` | `FocusEvent` | Callback to invoke when component loses focus. |
| `onChange` | `ListboxChangeEvent` | Callback to invoke on value change. |
| `onClick` | `ListboxClickEvent` | Callback to invoke when option is clicked. |
| `onDblClick` | `ListboxDoubleClickEvent` | Callback to invoke when option is double clicked. |
| `onDrop` | `CdkDragDrop<string[], string[], any>` | Emits on item is dropped. |
| `onFilter` | `ListboxFilterEvent` | Callback to invoke when data is filtered. |
| `onFocus` | `FocusEvent` | Callback to invoke when component receives focus. |
| `onLazyLoad` | `ScrollerLazyLoadEvent` | Emits on lazy load. |
| `onSelectAllChange` | `ListboxSelectAllChangeEvent` | Callback to invoke when all data is selected. |

#### Templates

Define with `<ng-template #name let-context>` inside the `p-listbox` element.

| Name | Context | Description |
|---|---|---|
| `item` | `ListboxItemTemplateContext<any>` | Custom item template. |
| `group` | `ListboxGroupTemplateContext<any>` | Custom group template. |
| `header` | `ListboxHeaderTemplateContext<any>` | Custom header template. |
| `filter` | `ListboxFilterTemplateContext` | Custom filter template. |
| `footer` | `ListboxFooterTemplateContext<any>` | Custom footer template. |
| `emptyfilter` | - | Custom empty filter message template. |
| `empty` | - | Custom empty message template. |
| `filtericon` | - | Custom filter icon template. |
| `checkicon` | `ListboxCheckIconTemplateContext` | Custom check icon template. |
| `checkmark` | `ListboxCheckmarkTemplateContext` | Custom checkmark icon template. |
| `loader` | `ListboxLoaderTemplateContext` | Custom loader template. |

#### Methods

| Name | Parameters | Returns | Description |
|---|---|---|---|
| `updateModel` | `value: any`, `event: any` | `void` | Updates the model value. |

## Theming

### CSS Classes

| Class | Description |
|---|---|
| `p-listbox` | Class name of the root element |
| `p-listbox-header` | Class name of the header element |
| `p-listbox-filter` | Class name of the filter element |
| `p-listbox-list-container` | Class name of the list container element |
| `p-listbox-list` | Class name of the list element |
| `p-listbox-option-group` | Class name of the option group element |
| `p-listbox-option` | Class name of the option element |
| `p-listbox-option-check-icon` | Class name of the option check icon element |
| `p-listbox-option-blank-icon` | Class name of the option blank icon element |
| `p-listbox-empty-message` | Class name of the empty message element |

### Design Tokens

| Token | CSS Variable | Description |
|---|---|---|
| `listbox.background` | `--p-listbox-background` | Background of root |
| `listbox.disabled.background` | `--p-listbox-disabled-background` | Disabled background of root |
| `listbox.border.color` | `--p-listbox-border-color` | Border color of root |
| `listbox.invalid.border.color` | `--p-listbox-invalid-border-color` | Invalid border color of root |
| `listbox.color` | `--p-listbox-color` | Color of root |
| `listbox.disabled.color` | `--p-listbox-disabled-color` | Disabled color of root |
| `listbox.shadow` | `--p-listbox-shadow` | Shadow of root |
| `listbox.border.radius` | `--p-listbox-border-radius` | Border radius of root |
| `listbox.transition.duration` | `--p-listbox-transition-duration` | Transition duration of root |
| `listbox.list.padding` | `--p-listbox-list-padding` | Padding of list |
| `listbox.list.gap` | `--p-listbox-list-gap` | Gap of list |
| `listbox.list.header.padding` | `--p-listbox-list-header-padding` | Header padding of list |
| `listbox.option.focus.background` | `--p-listbox-option-focus-background` | Focus background of option |
| `listbox.option.selected.background` | `--p-listbox-option-selected-background` | Selected background of option |
| `listbox.option.selected.focus.background` | `--p-listbox-option-selected-focus-background` | Selected focus background of option |
| `listbox.option.color` | `--p-listbox-option-color` | Color of option |
| `listbox.option.focus.color` | `--p-listbox-option-focus-color` | Focus color of option |
| `listbox.option.selected.color` | `--p-listbox-option-selected-color` | Selected color of option |
| `listbox.option.selected.focus.color` | `--p-listbox-option-selected-focus-color` | Selected focus color of option |
| `listbox.option.padding` | `--p-listbox-option-padding` | Padding of option |
| `listbox.option.border.radius` | `--p-listbox-option-border-radius` | Border radius of option |
| `listbox.option.striped.background` | `--p-listbox-option-striped-background` | Striped background of option |
| `listbox.option.group.background` | `--p-listbox-option-group-background` | Background of option group |
| `listbox.option.group.color` | `--p-listbox-option-group-color` | Color of option group |
| `listbox.option.group.font.weight` | `--p-listbox-option-group-font-weight` | Font weight of option group |
| `listbox.option.group.padding` | `--p-listbox-option-group-padding` | Padding of option group |
| `listbox.checkmark.color` | `--p-listbox-checkmark-color` | Color of checkmark |
| `listbox.checkmark.gutter.start` | `--p-listbox-checkmark-gutter-start` | Gutter start of checkmark |
| `listbox.checkmark.gutter.end` | `--p-listbox-checkmark-gutter-end` | Gutter end of checkmark |
| `listbox.empty.message.padding` | `--p-listbox-empty-message-padding` | Padding of empty message |
