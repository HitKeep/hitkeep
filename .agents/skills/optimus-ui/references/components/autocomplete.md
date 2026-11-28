# AutoComplete

AutoComplete is an input component that provides real-time suggestions when being typed.

## Import

```typescript
import { AutoCompleteModule, AutoComplete } from '@openng/optimus-ui/autocomplete';
```

## Basic

AutoComplete uses ngModel for two-way binding, requires a list of suggestions and a completeMethod to query for the results. The completeMethod gets the query text as event.query property and should update the suggestions with the search results.

```typescript
import { Component } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { AutoCompleteModule } from '@openng/optimus-ui/autocomplete';

interface AutoCompleteCompleteEvent {
    originalEvent: Event;
    query: string;
}

@Component({
    template: `
        <div class="card flex justify-center">
            <p-autocomplete [(ngModel)]="value" [suggestions]="items" (completeMethod)="search($event)" />
        </div>
    `,
    standalone: true,
    imports: [AutoCompleteModule, FormsModule]
})
export class AutocompleteBasicDemo {
    items: any[] = [];
    value: any;

    search(event: AutoCompleteCompleteEvent) {
        this.items = [...Array(10).keys()].map((item) => event.query + '-' + item);
    }
}
```

## Dropdown

Enabling dropdown property displays a button next to the input field where click behavior of the button is defined using dropdownMode property that takes blank or current as possible values. blank is the default mode to send a query with an empty string whereas current setting sends a query with the current value of the input.

```typescript
import { Component } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { AutoCompleteModule } from '@openng/optimus-ui/autocomplete';

interface AutoCompleteCompleteEvent {
    originalEvent: Event;
    query: string;
}

@Component({
    template: `
        <div class="card flex justify-center">
            <p-autocomplete [(ngModel)]="value" [dropdown]="true" [suggestions]="items" (completeMethod)="search($event)" />
        </div>
    `,
    standalone: true,
    imports: [AutoCompleteModule, FormsModule]
})
export class AutocompleteDropdownDemo {
    items: any[] | undefined;
    value: any;

    search(event: AutoCompleteCompleteEvent) {
        let _items = [...Array(10).keys()];
        
        this.items = event.query ? [...Array(10).keys()].map((item) => event.query + '-' + item) : _items;
    }
}
```

## Objects

AutoComplete can also work with objects using the optionLabel property that defines the label to display as a suggestion. The value passed to the model would still be the object instance of a suggestion. Here is an example with a Country object that has name and code fields such as &#123;name: "United States", code:"USA"&#125; .

```typescript
import { Component, OnInit, inject } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { AutoCompleteModule } from '@openng/optimus-ui/autocomplete';
import { CountryService } from '@/service/countryservice';
import { Country } from '@/domain/customer';

interface AutoCompleteCompleteEvent {
    originalEvent: Event;
    query: string;
}

@Component({
    template: `
        <div class="card flex justify-center">
            <p-autocomplete [(ngModel)]="selectedCountry" [suggestions]="filteredCountries" (completeMethod)="filterCountry($event)" optionLabel="name" />
        </div>
    `,
    standalone: true,
    imports: [AutoCompleteModule, FormsModule],
    providers: [CountryService]
})
export class AutocompleteObjectsDemo implements OnInit {
    private countryService = inject(CountryService);
    countries: any[] | undefined;
    selectedCountry: any;
    filteredCountries: any[] | undefined;

    ngOnInit() {
        this.countryService.getCountries().then((countries) => {
            this.countries = countries;
        });
    }

    filterCountry(event: AutoCompleteCompleteEvent) {
        let filtered: any[] = [];
        let query = event.query;
        
        for (let i = 0; i < (this.countries as any[]).length; i++) {
            let country = (this.countries as any[])[i];
            if (country.name.toLowerCase().indexOf(query.toLowerCase()) == 0) {
                filtered.push(country);
            }
        }
        
        this.filteredCountries = filtered;
    }
}
```

## Template

AutoComplete offers multiple templates for customization through templating.

```typescript
import { Component, OnInit, inject } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { AutoCompleteModule } from '@openng/optimus-ui/autocomplete';
import { ButtonModule } from '@openng/optimus-ui/button';
import { CountryService } from '@/service/countryservice';

interface AutoCompleteCompleteEvent {
    originalEvent: Event;
    query: string;
}

@Component({
    template: `
        <div class="card flex justify-center">
            <p-autocomplete [(ngModel)]="selectedCountryAdvanced" [suggestions]="filteredCountries" (completeMethod)="filterCountry($event)" optionLabel="name">
                <ng-template let-country #item>
                    <div class="flex items-center gap-2">
                        <img src="https://optimus.openng.org/demo/flag/flag_placeholder.png" [class]="'flag flag-' + country.code.toLowerCase()" style="width: 18px" />
                        <div>{{ country.name }}</div>
                    </div>
                </ng-template>
                <ng-template #header>
                    <div class="font-medium px-3 py-2">Available Countries</div>
                </ng-template>
                <ng-template #footer>
                    <div class="px-3 py-3">
                        <p-button label="Add New" fluid severity="secondary" text size="small" icon="pi pi-plus" />
                    </div>
                </ng-template>
            </p-autocomplete>
        </div>
    `,
    standalone: true,
    imports: [AutoCompleteModule, ButtonModule, FormsModule],
    providers: [CountryService]
})
export class AutocompleteTemplateDemo implements OnInit {
    private countryService = inject(CountryService);
    countries: any[] | undefined;
    selectedCountryAdvanced: any[] | undefined;
    filteredCountries: any[] | undefined;

    ngOnInit() {
        this.countryService.getCountries().then((countries) => {
            this.countries = countries;
        });
    }

    filterCountry(event: AutoCompleteCompleteEvent) {
        let filtered: any[] = [];
        let query = event.query;
        
        for (let i = 0; i < (this.countries as any[]).length; i++) {
            let country = (this.countries as any[])[i];
            if (country.name.toLowerCase().indexOf(query.toLowerCase()) == 0) {
                filtered.push(country);
            }
        }
        this.filteredCountries = filtered;
    }
}
```

## Group

Option grouping is enabled when group property is set to true . group template is available to customize the option groups. All templates get the option instance as the default local template variable.

```typescript
import { Component, OnInit } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { AutoCompleteModule } from '@openng/optimus-ui/autocomplete';
import { SelectItemGroup, FilterService } from '@openng/optimus-ui/api';

interface AutoCompleteCompleteEvent {
    originalEvent: Event;
    query: string;
}

@Component({
    template: `
        <div class="card flex justify-center">
            <p-autocomplete [(ngModel)]="selectedCity" [group]="true" [suggestions]="filteredGroups" (completeMethod)="filterGroupedCity($event)" placeholder="Hint: type 'a'">
                <ng-template let-group #group>
                    <div class="flex items-center">
                        <img src="https://optimus.openng.org/demo/flag/flag_placeholder.png" [class]="'mr-2 flag flag-' + group.value" style="width: 20px" />
                        <span>{{ group.label }}</span>
                    </div>
                </ng-template>
            </p-autocomplete>
        </div>
    `,
    standalone: true,
    imports: [AutoCompleteModule, FormsModule]
})
export class AutocompleteGroupDemo implements OnInit {
    selectedCity: any;
    filteredGroups: any[] | undefined;
    groupedCities: SelectItemGroup[] | undefined;

    ngOnInit() {
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

    filterGroupedCity(event: AutoCompleteCompleteEvent) {
        let query = event.query;
        let filteredGroups = [];
        
        for (let optgroup of this.groupedCities as SelectItemGroup[]) {
            let filteredSubOptions = this.filterService.filter(optgroup.items, ['label'], query, 'contains');
            if (filteredSubOptions && filteredSubOptions.length) {
                filteredGroups.push({
                    label: optgroup.label,
                    value: optgroup.value,
                    items: filteredSubOptions
                });
            }
        }
        
        this.filteredGroups = filteredGroups;
    }
}
```

## Multiple

Enable multiple selection mode using the ⁠multiple property to allow users to select more than one value from the autocomplete. When enabled, the value reference must be an array.

```typescript
import { Component } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { AutoCompleteModule } from '@openng/optimus-ui/autocomplete';

interface AutoCompleteCompleteEvent {
    originalEvent: Event;
    query: string;
}

@Component({
    template: `
        <div class="card">
            <label for="multiple-ac-1" class="font-bold mb-2 block">With Typeahead</label>
            <p-autocomplete [(ngModel)]="value1" inputId="multiple-ac-1" multiple fluid [suggestions]="items" (completeMethod)="search($event)" />
            <label for="multiple-ac-2" class="font-bold mt-8 mb-2 block">Without Typeahead</label>
            <p-autocomplete [(ngModel)]="value2" inputId="multiple-ac-2" multiple fluid (completeMethod)="search($event)" [typeahead]="false" />
        </div>
    `,
    standalone: true,
    imports: [AutoCompleteModule, FormsModule]
})
export class AutocompleteMultipleDemo {
    value1: any[] | undefined;
    value2: any[] | undefined;
    items: any[] | undefined;

    search(event: AutoCompleteCompleteEvent) {
        this.items = [...Array(10).keys()].map((item) => event.query + '-' + item);
    }
}
```

## Sizes

AutoComplete provides small and large sizes as alternatives to the base.

```typescript
import { Component } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { AutoCompleteModule } from '@openng/optimus-ui/autocomplete';

@Component({
    template: `
        <div class="card flex flex-col items-center gap-4">
            <p-autocomplete [(ngModel)]="value1" [suggestions]="items" (completeMethod)="search()" size="small" placeholder="Small" dropdown />
            <p-autocomplete [(ngModel)]="value2" [suggestions]="items" (completeMethod)="search()" placeholder="Normal" dropdown />
            <p-autocomplete [(ngModel)]="value3" [suggestions]="items" (completeMethod)="search()" size="large" placeholder="Large" dropdown />
        </div>
    `,
    standalone: true,
    imports: [AutoCompleteModule, FormsModule]
})
export class AutocompleteSizesDemo {
    items: any[] | undefined;
    value1: any;
    value2: any;
    value3: any;

    search() {
        this.items = [];
    }
}
```

## Fluid

The fluid prop makes the component take up the full width of its container when set to true.

```typescript
import { Component } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { AutoCompleteModule } from '@openng/optimus-ui/autocomplete';

interface AutoCompleteCompleteEvent {
    originalEvent: Event;
    query: string;
}

@Component({
    template: `
        <div class="card">
            <p-autocomplete [(ngModel)]="value" [suggestions]="items" (completeMethod)="search($event)" fluid />
        </div>
    `,
    standalone: true,
    imports: [AutoCompleteModule, FormsModule]
})
export class AutocompleteFluidDemo {
    items: any[] = [];
    value: any;

    search(event: AutoCompleteCompleteEvent) {
        this.items = [...Array(10).keys()].map((item) => event.query + '-' + item);
    }
}
```

## Filled

Specify the variant property as filled to display the component with a higher visual emphasis than the default outlined style.

```typescript
import { Component } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { AutoCompleteModule } from '@openng/optimus-ui/autocomplete';

interface AutoCompleteCompleteEvent {
    originalEvent: Event;
    query: string;
}

@Component({
    template: `
        <div class="card flex justify-center">
            <p-autocomplete [(ngModel)]="selectedItem" [suggestions]="suggestions" (completeMethod)="search($event)" variant="filled" />
        </div>
    `,
    standalone: true,
    imports: [AutoCompleteModule, FormsModule]
})
export class AutocompleteFilledDemo {
    items: any[] | undefined;
    selectedItem: any;
    suggestions: any[] | undefined;

    search(event: AutoCompleteCompleteEvent) {
        this.suggestions = [...Array(10).keys()].map((item) => event.query + '-' + item);
    }
}
```

## Disabled

When disabled is present, the element cannot be edited and focused.

```typescript
import { Component } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { AutoCompleteModule } from '@openng/optimus-ui/autocomplete';

interface AutoCompleteCompleteEvent {
    originalEvent: Event;
    query: string;
}

@Component({
    template: `
        <div class="card flex justify-center">
            <p-autocomplete [(ngModel)]="selectedItem" [suggestions]="suggestions" placeholder="Disabled" (completeMethod)="search($event)" [disabled]="true" />
        </div>
    `,
    standalone: true,
    imports: [AutoCompleteModule, FormsModule]
})
export class AutocompleteDisabledDemo {
    items: any[] | undefined;
    selectedItem: any;
    suggestions: any[] | undefined;

    search(event: AutoCompleteCompleteEvent) {
        this.suggestions = [...Array(10).keys()].map((item) => event.query + '-' + item);
    }
}
```

## Invalid

The invalid state is applied using the ⁠invalid property to indicate failed validation, which can be integrated with Angular Forms.

```typescript
import { Component } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { AutoCompleteModule } from '@openng/optimus-ui/autocomplete';

interface AutoCompleteCompleteEvent {
    originalEvent: Event;
    query: string;
}

@Component({
    template: `
        <div class="card flex flex-wrap justify-center gap-4">
            <p-autocomplete [(ngModel)]="value1" [suggestions]="suggestions" [invalid]="!value1" (completeMethod)="search($event)" placeholder="Code" />
            <p-autocomplete [(ngModel)]="value2" [suggestions]="suggestions" [invalid]="!value2" (completeMethod)="search($event)" variant="filled" placeholder="Code" />
        </div>
    `,
    standalone: true,
    imports: [AutoCompleteModule, FormsModule]
})
export class AutocompleteInvalidDemo {
    value1: any;
    value2: any;
    suggestions: any[] | undefined;

    search(event: AutoCompleteCompleteEvent) {
        this.suggestions = [...Array(10).keys()].map((item) => event.query + '-' + item);
    }
}
```

## Accessibility

Screen Reader Value to describe the component can either be provided via label tag combined with inputId prop or using ariaLabelledBy , ariaLabel props. The input element has combobox role in addition to aria-autocomplete , aria-haspopup and aria-expanded attributes. The relation between the input and the popup is created with aria-controls and aria-activedescendant attribute is used to instruct screen reader which option to read during keyboard navigation within the popup list. In multiple mode, chip list uses listbox role whereas each chip has the option role with aria-label set to the label of the chip. The popup list has an id that refers to the aria-controls attribute of the input element and uses listbox as the role. Each list item has option role and an id to match the aria-activedescendant of the input element.

## Advanced-chips

This example demonstrates an advanced use case with templating, object handling, dropdown, and multiple mode.

```typescript
import { Component, OnInit, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { AutoCompleteModule } from '@openng/optimus-ui/autocomplete';
import { ProductService } from '@/service/productservice';
import { Product } from '@/domain/product';

@Component({
    template: `
        <div class="card">
            <p-autocomplete
                [(ngModel)]="selectedProducts"
                [suggestions]="filteredProducts"
                (completeMethod)="filterProducts($event)"
                inputId="advanced-chips"
                [multiple]="true"
                [fluid]="true"
                [typeahead]="false"
                [dropdown]="true"
                [optionLabel]="getProductLabel"
                [optionValue]="getProductValue"
                placeholder="Add products"
            >
                <ng-template let-product #item>
                    <div class="flex flex-wrap p-1 items-center gap-4 w-full">
                        <img class="w-12 shrink-0 rounded" src="https://optimus.openng.org/demo/product/{{ product.image }}" [alt]="product.name" />
                        <div class="flex-1 flex flex-col">
                            <span class="font-medium text-sm">{{ product.name }}</span>
                            <span class="text-sm text-surface-500 dark:text-surface-400">{{ product.category }}</span>
                        </div>
                        <span class="font-bold sm:ml-8">\${{ product.price }}</span>
                    </div>
                </ng-template>
                <ng-template let-value #selecteditem>
                    @if (value.price) {
                        <div class="flex align-items-center gap-2">
                            <span class="font-semibold">{{ value.name }}</span>
                            <span class="text-primary text-sm font-bold">\${{ value.price }}</span>
                        </div>
                    } @else {
                        <span class="font-semibold">{{ value }}</span>
                    }
                </ng-template>
            </p-autocomplete>
        </div>
    `,
    standalone: true,
    imports: [AutoCompleteModule, FormsModule],
    providers: [ProductService]
})
export class AutocompleteAdvancedChipsDemo implements OnInit {
    private productService = inject(ProductService);
    products = signal<Product[]>([]);

    ngOnInit() {
        this.productService.getProductsSmall().then((data) => this.products.set(data));
    }

    filterProducts(event: any) {
        let filtered: Product[] = [];
        let query = event.query;
        
        for (let i = 0; i < this.products().length; i++) {
            let product = this.products()[i];
            if (product.name?.toLowerCase().indexOf(query.toLowerCase()) == 0) {
                filtered.push(product);
            }
        }
        
        this.filteredProducts = filtered;
    }

    getProductLabel(product: any): string {
        if (typeof product === 'string') {
            return product;
        }
        return product?.name || '';
    }

    getProductValue(product: any): any {
        if (typeof product === 'string') {
            return { name: product, custom: true };
        }
        return {
            id: product.id,
            name: product.name,
            price: product.price,
            quantity: product.quantity
        };
    }
}
```

## Basic-chips

With ⁠multiple enabled, the AutoComplete component behaves like a chips or tags input. Use addOnBlur , ⁠addOnTab , and ⁠separator properties to customize the keystroke behavior for adding items.

```typescript
import { Component } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { AutoCompleteModule } from '@openng/optimus-ui/autocomplete';

@Component({
    template: `
        <div class="card">
            <label for="chips-blur" class="font-bold mb-2 block">With Add On Blur</label>
            <p-autocomplete [(ngModel)]="valueBlur" inputId="chips-blur" multiple fluid [typeahead]="false" [addOnBlur]="true" placeholder="Type and click outside to add..." />
            <label for="chips-tab" class="font-bold mt-8 mb-2 block">With Add On Tab</label>
            <p-autocomplete [(ngModel)]="valueTab" inputId="chips-tab" multiple fluid [typeahead]="false" [addOnTab]="true" placeholder="Type and press Tab to add..." />
            <label for="chips-separator" class="font-bold mt-8 mb-2 block">With Separator (Comma)</label>
            <p-autocomplete [(ngModel)]="valueSeparator" inputId="chips-separator" multiple fluid [typeahead]="false" separator="," placeholder="Type items separated by comma..." />
            <label for="chips-combined" class="font-bold mt-8 mb-2 block">Combined Features</label>
            <p-autocomplete [(ngModel)]="valueCombined" inputId="chips-combined" multiple fluid [typeahead]="false" [addOnBlur]="true" [addOnTab]="true" separator="," placeholder="Use Tab, Blur, or Comma to add items..." />
        </div>
    `,
    standalone: true,
    imports: [AutoCompleteModule, FormsModule]
})
export class AutocompleteBasicChipsDemo {
    valueBlur: any[] = [];
    valueTab: any[] = [];
    valueSeparator: any[] = [];
    valueCombined: any[] = [];
}
```

## Clear-icon

When showClear is enabled, a clear icon is displayed to clear the value.

```typescript
import { Component } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { AutoCompleteModule } from '@openng/optimus-ui/autocomplete';

interface AutoCompleteCompleteEvent {
    originalEvent: Event;
    query: string;
}

@Component({
    template: `
        <div class="card flex justify-center">
            <p-autocomplete [(ngModel)]="value" [suggestions]="items" (completeMethod)="search($event)" [showClear]="true" inputStyleClass="w-56" />
        </div>
    `,
    standalone: true,
    imports: [AutoCompleteModule, FormsModule]
})
export class AutocompleteClearIconDemo {
    items: any[] = [];
    value: any;

    search(event: AutoCompleteCompleteEvent) {
        this.items = [...Array(10).keys()].map((item) => event.query + '-' + item);
    }
}
```

## Float-label

A floating label appears on top of the input field when focused. Visit FloatLabel documentation for more information.

```typescript
import { Component } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { AutoCompleteModule } from '@openng/optimus-ui/autocomplete';
import { FloatLabelModule } from '@openng/optimus-ui/floatlabel';

interface AutoCompleteCompleteEvent {
    originalEvent: Event;
    query: string;
}

@Component({
    template: `
        <div class="card flex flex-wrap justify-center items-end gap-4">
            <p-floatlabel>
                <p-autocomplete [(ngModel)]="value1" [suggestions]="items" (completeMethod)="search($event)" inputId="over_label" />
                <label for="over_label">Over Label</label>
            </p-floatlabel>
            <p-floatlabel variant="in">
                <p-autocomplete [(ngModel)]="value2" [suggestions]="items" (completeMethod)="search($event)" inputId="in_label" />
                <label for="in_label">In Label</label>
            </p-floatlabel>
            <p-floatlabel variant="on">
                <p-autocomplete [(ngModel)]="value3" [suggestions]="items" (completeMethod)="search($event)" inputId="on_label" />
                <label for="on_label">On Label</label>
            </p-floatlabel>
        </div>
    `,
    standalone: true,
    imports: [AutoCompleteModule, FloatLabelModule, FormsModule]
})
export class AutocompleteFloatLabelDemo {
    value1: string | undefined;
    value2: string | undefined;
    value3: string | undefined;
    items: any[] | undefined;

    search(event: AutoCompleteCompleteEvent) {
        this.items = [...Array(10).keys()].map((item) => event.query + '-' + item);
    }
}
```

## Force-selection

ForceSelection mode validates the manual input to check whether it also exists in the suggestions list, if not the input value is cleared to make sure the value passed to the model is always one of the suggestions.

```typescript
import { Component, OnInit, inject } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { AutoCompleteModule } from '@openng/optimus-ui/autocomplete';
import { CountryService } from '@/service/countryservice';

interface AutoCompleteCompleteEvent {
    originalEvent: Event;
    query: string;
}

@Component({
    template: `
        <div class="card flex justify-center">
            <p-autocomplete [(ngModel)]="selectedCountry" [forceSelection]="true" [suggestions]="filteredCountries" (completeMethod)="filterCountry($event)" optionLabel="name" />
        </div>
    `,
    standalone: true,
    imports: [AutoCompleteModule, FormsModule],
    providers: [CountryService]
})
export class AutocompleteForceSelectionDemo implements OnInit {
    private countryService = inject(CountryService);
    countries: any[] | undefined;
    selectedCountry: any;
    filteredCountries: any[] | undefined;

    ngOnInit() {
        this.countryService.getCountries().then((countries) => {
            this.countries = countries;
        });
    }

    filterCountry(event: AutoCompleteCompleteEvent) {
        let filtered: any[] = [];
        let query = event.query;
        
        for (let i = 0; i < (this.countries as any[]).length; i++) {
            let country = (this.countries as any[])[i];
            if (country.name.toLowerCase().indexOf(query.toLowerCase()) == 0) {
                filtered.push(country);
            }
        }
        
        this.filteredCountries = filtered;
    }
}
```

## Ifta-label

IftaLabel is used to create infield top aligned labels. Visit IftaLabel documentation for more information.

```typescript
import { Component } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { AutoCompleteModule } from '@openng/optimus-ui/autocomplete';
import { IftaLabelModule } from '@openng/optimus-ui/iftalabel';

interface AutoCompleteCompleteEvent {
    originalEvent: Event;
    query: string;
}

@Component({
    template: `
        <div class="card flex justify-center">
            <p-iftalabel>
                <p-autocomplete [(ngModel)]="value" [suggestions]="items" (completeMethod)="search($event)" inputId="ac" />
                <label for="ac">Identifier</label>
            </p-iftalabel>
        </div>
    `,
    standalone: true,
    imports: [AutoCompleteModule, IftaLabelModule, FormsModule]
})
export class AutocompleteIftaLabelDemo {
    items: any[] | undefined;
    value: any;

    search(event: AutoCompleteCompleteEvent) {
        this.items = [...Array(10).keys()].map((item) => event.query + '-' + item);
    }
}
```

## Reactive-forms

AutoComplete can also be used with reactive forms. In this case, the formControlName property is used to bind the component to a form control.

```typescript
import { Component, inject } from '@angular/core';
import { ReactiveFormsModule } from '@angular/forms';
import { AutoCompleteModule } from '@openng/optimus-ui/autocomplete';
import { MessageModule } from '@openng/optimus-ui/message';
import { ToastModule } from '@openng/optimus-ui/toast';
import { ButtonModule } from '@openng/optimus-ui/button';
import { MessageService } from '@openng/optimus-ui/api';

interface AutoCompleteCompleteEvent {
    originalEvent: Event;
    query: string;
}

@Component({
    template: `
        <p-toast />
        <div class="card flex justify-center">
            <form [formGroup]="exampleForm" (ngSubmit)="onSubmit()" class="flex justify-center flex-col gap-4 md:w-56">
                <div class="flex flex-col gap-1">
                    <p-autocomplete formControlName="value" [suggestions]="items" [invalid]="isInvalid('value')" (completeMethod)="search($event)" fluid />
                    @if (isInvalid('value')) {
                        <p-message severity="error" size="small" variant="simple">Value is required.</p-message>
                    }
                </div>
                <button pButton severity="secondary" type="submit"><span pButtonLabel>Submit</span></button>
            </form>
        </div>
    `,
    standalone: true,
    imports: [AutoCompleteModule, MessageModule, ToastModule, ButtonModule, ReactiveFormsModule]
})
export class AutocompleteReactiveFormsDemo {
    messageService = inject(MessageService);
    items: any[] | undefined;
    exampleForm: FormGroup | undefined;
    formSubmitted: boolean = false;

    constructor() {
        this.exampleForm = this.fb.group({
                    value: ['', Validators.required]
                });
    }

    search(event: AutoCompleteCompleteEvent) {
        this.items = [...Array(10).keys()].map((item) => event.query + '-' + item);
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

## Template-driven-forms

```typescript
import { Component, inject } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { AutoCompleteModule } from '@openng/optimus-ui/autocomplete';
import { MessageModule } from '@openng/optimus-ui/message';
import { ToastModule } from '@openng/optimus-ui/toast';
import { ButtonModule } from '@openng/optimus-ui/button';
import { MessageService } from '@openng/optimus-ui/api';

@Component({
    template: `
        <p-toast />
        <div class="card flex justify-center">
            <form #exampleForm="ngForm" (ngSubmit)="onSubmit(exampleForm)" class="flex justify-center flex-col gap-4 md:w-56">
                <div class="flex flex-col gap-1">
                    <p-autocomplete #val="ngModel" [(ngModel)]="value" [suggestions]="items" [invalid]="val.invalid && (val.touched || exampleForm.submitted)" name="val" (completeMethod)="search($event)" required fluid />
                    @if (val.invalid && (val.touched || exampleForm.submitted)) {
                        <p-message severity="error" size="small" variant="simple">Value is required.</p-message>
                    }
                </div>
                <button pButton severity="secondary" type="submit"><span pButtonLabel>Submit</span></button>
            </form>
        </div>
    `,
    standalone: true,
    imports: [AutoCompleteModule, MessageModule, ToastModule, ButtonModule, FormsModule]
})
export class AutocompleteTemplateDrivenFormsDemo {
    messageService = inject(MessageService);
    items: any[] = [];
    value: any;

    search(event: AutoCompleteCompleteEvent) {
        this.items = [...Array(10).keys()].map((item) => event.query + '-' + item);
    }

    onSubmit(form: any) {
        if (form.valid) {
            this.messageService.add({ severity: 'success', summary: 'Success', detail: 'Form Submitted', life: 3000 });
            form.resetForm();
        }
    }
}
```

## Virtual-scroll

Virtual scrolling is an efficient way of rendering the options by displaying a small subset of data in the viewport at any time. When dealing with huge number of options, it is suggested to enable virtual scrolling to avoid performance issues. Usage is simple as setting virtualScroll property to true and defining virtualScrollItemSize to specify the height of an item.

```typescript
import { Component, OnInit } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { AutoCompleteModule } from '@openng/optimus-ui/autocomplete';

interface AutoCompleteCompleteEvent {
    originalEvent: Event;
    query: string;
}

@Component({
    template: `
        <div class="card flex justify-center">
            <p-autocomplete [(ngModel)]="selectedItem" [virtualScroll]="true" [suggestions]="filteredItems" [virtualScrollItemSize]="34" (completeMethod)="filterItems($event)" optionLabel="label" [dropdown]="true" />
        </div>
    `,
    standalone: true,
    imports: [AutoCompleteModule, FormsModule]
})
export class AutocompleteVirtualScrollDemo implements OnInit {
    selectedItem: any;
    filteredItems: any[] | undefined;
    items: any[] | undefined;

    ngOnInit() {
        this.items = [];
        for (let i = 0; i < 10000; i++) {
            this.items.push({ label: 'Item ' + i, value: 'Item ' + i });
        }
    }

    filterItems(event: AutoCompleteCompleteEvent) {
        //in a real application, make a request to a remote url with the query and return filtered results, for demo we filter at client side
        let filtered: any[] = [];
        let query = event.query;
        
        for (let i = 0; i < (this.items as any[]).length; i++) {
            let item = (this.items as any[])[i];
            if (item.label.toLowerCase().indexOf(query.toLowerCase()) == 0) {
                filtered.push(item);
            }
        }
        
        this.filteredItems = filtered;
    }
}
```

## API

### AutoComplete

Selector: `p-autoComplete, p-autocomplete, p-auto-complete` · Component, standalone · works with `ngModel`, `formControl` and `formControlName`

AutoComplete is an input component that provides real-time suggestions when being typed.

#### Inputs

| Name | Type | Default | Description |
|---|---|---|---|
| `addOnBlur` | `boolean` | `false` | Whether to add an item on blur event if the input has value and typeahead is false with multiple mode. |
| `addOnTab` | `boolean` | `false` | When enabled, the input value is added to the selected items on tab key press when multiple is true and typeahead is false. |
| `appendTo` | `any` | - | Target element to attach the overlay, valid values are "body" or a local ng-template variable of another element (note: use binding with brackets for template variables, e.g. [appendTo]="mydiv" for a div element having #mydiv as variable name). |
| `ariaLabel` | `string \| undefined` | - | Defines a string that labels the input for accessibility. |
| `ariaLabelledBy` | `string \| undefined` | - | Specifies one or more IDs in the DOM that labels the input field. |
| `autocomplete` | `string` | `'off'` | Used to define a string that autocomplete attribute the current element. |
| `autofocus` | `boolean \| undefined` | - | When present, it specifies that the component should automatically get focus on load. |
| `autoHighlight` | `boolean \| undefined` | - | When enabled, highlights the first item in the list by default. |
| `autoOptionFocus` | `boolean \| undefined` | `false` | Whether to focus on the first visible or selected element when the overlay panel is shown. |
| `autoZIndex` | `boolean` | `true` | Whether to automatically manage layering. |
| `baseZIndex` | `number` | `0` | Base zIndex value to use in layering. |
| `completeOnFocus` | `boolean` | `false` | Whether to run a query when input receives focus. |
| `dataKey` | `string \| undefined` | - | A property to uniquely identify a value in options. |
| `delay` | `number` | `300` | Delay between keystrokes to wait before sending a query. |
| `disabled` | `boolean \| undefined` | - | When present, it specifies that the component should have disabled state style. |
| `dropdown` | `boolean \| undefined` | - | Displays a button next to the input field when enabled. |
| `dropdownAriaLabel` | `string \| undefined` | - | Defines a string that labels the dropdown button for accessibility. |
| `dropdownIcon` | `string \| undefined` | - | Icon class of the dropdown icon. |
| `dropdownMode` | `string` | `'blank'` | Specifies the behavior dropdown button. Default "blank" mode sends an empty string and "current" mode sends the input value. |
| `dt` | `Object \| undefined` | - | Defines scoped design tokens of the component. |
| `emptyMessage` | `string \| undefined` | - | Text to display when there is no data. Defaults to global value in i18n translation configuration. |
| `emptySelectionMessage` | `string \| undefined` | `'No selected item'` | Text to display when filtering does not return any results. Defaults to global value in i18n translation configuration. |
| `fluid` | `boolean \| undefined` | - | Spans 100% width of the container when enabled. |
| `focusOnHover` | `boolean \| undefined` | `true` | When enabled, the hovered option will be focused. |
| `forceSelection` | `boolean \| undefined` | - | When present, autocomplete clears the manual input if it does not match of the suggestions to force only accepting values from the suggestions. |
| `group` | `boolean \| undefined` | - | Whether to display options as grouped when nested options are provided. |
| `hideTransitionOptions` | `string` | `'.1s linear'` | Transition options of the hide animation. **Deprecated**: since v21.0.0, use `motionOptions` instead. |
| `id` | `string \| undefined` | - | Unique identifier of the component. |
| `inputId` | `string \| undefined` | - | Identifier of the focus input to match a label defined for the component. |
| `inputSize` | `number \| null \| undefined` | - | Specifies the visible width of the input element in characters. |
| `inputStyle` | `{ [klass: string]: any; } \| null \| undefined` | - | Inline style of the input field. |
| `inputStyleClass` | `string \| undefined` | - | Inline style of the input field. |
| `invalid` | `boolean \| undefined` | - | When present, it specifies that the component should have invalid state style. |
| `lazy` | `boolean` | `false` | Defines if data is loaded and interacted with in lazy manner. |
| `max` | `number \| null \| undefined` | - | The value must be less than or equal to the value. |
| `maxlength` | `number \| null \| undefined` | - | The number of characters (code points) must not exceed the value of the attribute. |
| `min` | `number \| null \| undefined` | - | The value must be greater than or equal to the value. |
| `minlength` | `number \| null \| undefined` | - | The number of characters (code points) must not be less than the value of the attribute, if non-empty. |
| `minLength` | `number` | `1` | Minimum number of characters to initiate a search. **Deprecated**: since v20.0.0, use `minQueryLength` instead. |
| `minQueryLength` | `number \| undefined` | - | Minimum number of characters to initiate a search. |
| `motionOptions` | `MotionOptions \| undefined` | - | The motion options. |
| `multiple` | `boolean \| undefined` | - | Specifies if multiple values can be selected. |
| `name` | `string \| undefined` | - | When present, it specifies that the name of the input. |
| `optionDisabled` | `string \| ((item: T) => string) \| undefined` | - | Property name or getter function to use as the disabled flag of an option, defaults to false when not defined. |
| `optionGroupChildren` | `string \| undefined` | `'items'` | Name of the options field of an option group. |
| `optionGroupLabel` | `string \| undefined` | `'label'` | Name of the label field of an option group. |
| `optionLabel` | `string \| ((item: T) => string) \| undefined` | - | Property name or getter function to use as the label of an option. |
| `optionValue` | `string \| ((item: T) => string) \| undefined` | - | Property name or getter function to use as the value of an option. |
| `overlayOptions` | `OverlayOptions \| undefined` | - | Options for the overlay element. |
| `panelStyle` | `{ [klass: string]: any; } \| null \| undefined` | - | Inline style of the overlay panel element. |
| `panelStyleClass` | `string \| undefined` | - | Style class of the overlay panel element. |
| `pattern` | `string \| null \| undefined` | - | Specifies the value must match the pattern. |
| `placeholder` | `string \| undefined` | - | Hint text for the input field. |
| `pt` | `AutoCompletePassThrough` | - | Used to pass attributes to DOM elements inside the component. |
| `ptOptions` | `PassThroughOptions \| undefined` | - | Used to configure passthrough(pt) options of the component. |
| `readonly` | `boolean \| undefined` | - | When present, it specifies that the input cannot be typed. |
| `required` | `boolean \| undefined` | - | There must be a value (if set). |
| `scrollHeight` | `string` | `'200px'` | Maximum height of the suggestions panel. |
| `searchLocale` | `boolean \| undefined` | - | Locale to use in searching. The default locale is the host environment's current locale. |
| `searchMessage` | `string \| undefined` | `'{0} results are available'` | Text to display when the search is active. Defaults to global value in i18n translation configuration. |
| `selectionMessage` | `string \| undefined` | `'{0} items selected'` | Text to be displayed in hidden accessible field when options are selected. Defaults to global value in i18n translation configuration. |
| `selectOnFocus` | `boolean \| undefined` | - | When enabled, the focused option is selected. |
| `separator` | `string \| RegExp \| undefined` | - | Separator char to add item when typeahead is false and multiple mode is enabled. |
| `showClear` | `boolean` | `false` | When enabled, a clear icon is displayed to clear the value. |
| `showEmptyMessage` | `boolean \| undefined` | `true` | Whether to show the empty message or not. |
| `showTransitionOptions` | `string` | `'.12s cubic-bezier(0, 0, 0.2, 1)'` | Transition options of the show animation. **Deprecated**: since v21.0.0, use `motionOptions` instead. |
| `size` | `"large" \| "small"` | - | Specifies the size of the component. |
| `step` | `number \| null \| undefined` | - | Unless the step is set to the any literal, the value must be min + an integral multiple of the step. |
| `styleClass` | `string \| undefined` | - | Style class of the component. **Deprecated**: since v20.0.0, use `class` instead. |
| `suggestions` | `T[]` | - | An array of suggestions to display. |
| `tabindex` | `number \| undefined` | - | Index of the element in tabbing order. |
| `type` | `string` | `'text'` | Type of the input, defaults to "text". |
| `typeahead` | `boolean` | `true` | Whether typeahead is active or not. |
| `unique` | `boolean` | `true` | Ensures uniqueness of selected items on multiple mode. |
| `unstyled` | `boolean \| undefined` | - | Indicates whether the component should be rendered without styles. |
| `variant` | `"filled" \| "outlined"` | `'outlined'` | Specifies the input variant of the component. |
| `virtualScroll` | `boolean \| undefined` | - | Whether the data should be loaded on demand during scroll. |
| `virtualScrollItemSize` | `number \| undefined` | - | Height of an item in the list for VirtualScrolling. |
| `virtualScrollOptions` | `ScrollerOptions \| undefined` | - | Whether to use the scroller feature. The properties of scroller component can be used like an object in it. |

#### Outputs

| Name | Payload | Description |
|---|---|---|
| `completeMethod` | `AutoCompleteCompleteEvent` | Callback to invoke to search for suggestions. |
| `onAdd` | `AutoCompleteAddEvent<string>` | Callback to invoke when an item is added via addOnBlur or separator features. |
| `onBlur` | `Event` | Callback to invoke when the component loses focus. |
| `onClear` | `Event \| undefined` | Callback to invoke when clear button is clicked. |
| `onDropdownClick` | `AutoCompleteDropdownClickEvent` | Callback to invoke to when dropdown button is clicked. |
| `onFocus` | `Event` | Callback to invoke when the component receives focus. |
| `onHide` | `Event` | Callback to invoke on overlay is hidden. |
| `onInputKeydown` | `KeyboardEvent` | Callback to invoke on input key down. |
| `onKeyUp` | `KeyboardEvent` | Callback to invoke on input key up. |
| `onLazyLoad` | `AutoCompleteLazyLoadEvent` | Callback to invoke on lazy load data. |
| `onSelect` | `AutoCompleteSelectEvent<T>` | Callback to invoke when a suggestion is selected. |
| `onShow` | `Event` | Callback to invoke on overlay is shown. |
| `onUnselect` | `AutoCompleteUnselectEvent<T>` | Callback to invoke when a selected value is removed. |

#### Templates

Define with `<ng-template #name let-context>` inside the `p-autoComplete` element.

| Name | Context | Description |
|---|---|---|
| `item` | `AutoCompleteItemTemplateContext<any>` | Custom item template. |
| `empty` | - | Custom empty message template. |
| `header` | - | Custom header template. |
| `footer` | - | Custom footer template. |
| `selecteditem` | `AutoCompleteSelectedItemTemplateContext<any>` | Custom selected item template. |
| `group` | `AutoCompleteGroupTemplateContext<any>` | Custom group template. |
| `loader` | `AutoCompleteLoaderTemplateContext` | Custom loader template. |
| `removeicon` | `AutoCompleteRemoveIconTemplateContext` | Custom remove icon template. |
| `loadingicon` | - | Custom loading icon template. |
| `clearicon` | - | Custom clear icon template. |
| `dropdownicon` | - | Custom dropdown icon template. |

## Pass Through Options

| Name | Type | Description |
|---|---|---|
| `host` | `PassThroughOption<HTMLElement, I>` | Used to pass attributes to the host's DOM element. |
| `root` | `PassThroughOption<HTMLElement, I>` | Used to pass attributes to the root's DOM element. |
| `pcInputText` | `InputTextPassThrough` | Used to pass attributes to the InputText component. |
| `inputMultiple` | `PassThroughOption<HTMLUListElement, I>` | Used to pass attributes to the input multiple's DOM element. |
| `chipItem` | `PassThroughOption<HTMLLIElement, I>` | Used to pass attributes to the chip item's DOM element. |
| `pcChip` | `ChipPassThrough` | Used to pass attributes to the Chip component. |
| `chipIcon` | `PassThroughOption<HTMLSpanElement, I>` | Used to pass attributes to the chip icon's DOM element. |
| `inputChip` | `PassThroughOption<HTMLLIElement, I>` | Used to pass attributes to the input chip's DOM element. |
| `clearIcon` | `PassThroughOption<SVGElement, I>` | Used to pass attributes to the clear icon's DOM element. |
| `loader` | `PassThroughOption<SVGElement, I>` | Used to pass attributes to the loader's DOM element. |
| `dropdown` | `PassThroughOption<HTMLButtonElement, I>` | Used to pass attributes to the dropdown button's DOM element. |
| `pcOverlay` | `OverlayPassThrough` | Used to pass attributes to the Overlay component. |
| `overlay` | `PassThroughOption<HTMLDivElement, I>` | Used to pass attributes to the overlay's DOM element. |
| `listContainer` | `PassThroughOption<HTMLDivElement, I>` | Used to pass attributes to the list container's DOM element. |
| `pcScroller` | `VirtualScrollerPassThrough` | Used to pass attributes to the Scroller component. |
| `list` | `PassThroughOption<HTMLUListElement, I>` | Used to pass attributes to the list's DOM element. |
| `optionGroup` | `PassThroughOption<HTMLLIElement, I>` | Used to pass attributes to the option group's DOM element. |
| `option` | `PassThroughOption<HTMLLIElement, I>` | Used to pass attributes to the option's DOM element. |
| `emptyMessage` | `PassThroughOption<HTMLLIElement, I>` | Used to pass attributes to the empty message's DOM element. |

## Theming

### CSS Classes

| Class | Description |
|---|---|
| `p-autocomplete` | Class name of the root element |
| `p-autocomplete-input` | Class name of the input element |
| `p-autocomplete-input-multiple` | Class name of the input multiple element |
| `p-autocomplete-chip-item` | Class name of the chip item element |
| `p-autocomplete-chip` | Class name of the chip element |
| `p-autocomplete-chip-icon` | Class name of the chip icon element |
| `p-autocomplete-input-chip` | Class name of the input chip element |
| `p-autocomplete-loader` | Class name of the loader element |
| `p-autocomplete-dropdown` | Class name of the dropdown element |
| `p-autocomplete-overlay` | Class name of the panel element |
| `p-autocomplete-list` | Class name of the list element |
| `p-autocomplete-option-group` | Class name of the option group element |
| `p-autocomplete-option` | Class name of the option element |
| `p-autocomplete-empty-message` | Class name of the empty message element |
| `p-autocomplete-clear-icon` | Class name of the clear icon |

### Design Tokens

| Token | CSS Variable | Description |
|---|---|---|
| `autocomplete.background` | `--p-autocomplete-background` | Background of root |
| `autocomplete.disabled.background` | `--p-autocomplete-disabled-background` | Disabled background of root |
| `autocomplete.filled.background` | `--p-autocomplete-filled-background` | Filled background of root |
| `autocomplete.filled.hover.background` | `--p-autocomplete-filled-hover-background` | Filled hover background of root |
| `autocomplete.filled.focus.background` | `--p-autocomplete-filled-focus-background` | Filled focus background of root |
| `autocomplete.border.color` | `--p-autocomplete-border-color` | Border color of root |
| `autocomplete.hover.border.color` | `--p-autocomplete-hover-border-color` | Hover border color of root |
| `autocomplete.focus.border.color` | `--p-autocomplete-focus-border-color` | Focus border color of root |
| `autocomplete.invalid.border.color` | `--p-autocomplete-invalid-border-color` | Invalid border color of root |
| `autocomplete.color` | `--p-autocomplete-color` | Color of root |
| `autocomplete.disabled.color` | `--p-autocomplete-disabled-color` | Disabled color of root |
| `autocomplete.placeholder.color` | `--p-autocomplete-placeholder-color` | Placeholder color of root |
| `autocomplete.invalid.placeholder.color` | `--p-autocomplete-invalid-placeholder-color` | Invalid placeholder color of root |
| `autocomplete.shadow` | `--p-autocomplete-shadow` | Shadow of root |
| `autocomplete.padding.x` | `--p-autocomplete-padding-x` | Padding x of root |
| `autocomplete.padding.y` | `--p-autocomplete-padding-y` | Padding y of root |
| `autocomplete.border.radius` | `--p-autocomplete-border-radius` | Border radius of root |
| `autocomplete.focus.ring.width` | `--p-autocomplete-focus-ring-width` | Focus ring width of root |
| `autocomplete.focus.ring.style` | `--p-autocomplete-focus-ring-style` | Focus ring style of root |
| `autocomplete.focus.ring.color` | `--p-autocomplete-focus-ring-color` | Focus ring color of root |
| `autocomplete.focus.ring.offset` | `--p-autocomplete-focus-ring-offset` | Focus ring offset of root |
| `autocomplete.focus.ring.shadow` | `--p-autocomplete-focus-ring-shadow` | Focus ring shadow of root |
| `autocomplete.transition.duration` | `--p-autocomplete-transition-duration` | Transition duration of root |
| `autocomplete.overlay.background` | `--p-autocomplete-overlay-background` | Background of overlay |
| `autocomplete.overlay.border.color` | `--p-autocomplete-overlay-border-color` | Border color of overlay |
| `autocomplete.overlay.border.radius` | `--p-autocomplete-overlay-border-radius` | Border radius of overlay |
| `autocomplete.overlay.color` | `--p-autocomplete-overlay-color` | Color of overlay |
| `autocomplete.overlay.shadow` | `--p-autocomplete-overlay-shadow` | Shadow of overlay |
| `autocomplete.list.padding` | `--p-autocomplete-list-padding` | Padding of list |
| `autocomplete.list.gap` | `--p-autocomplete-list-gap` | Gap of list |
| `autocomplete.option.focus.background` | `--p-autocomplete-option-focus-background` | Focus background of option |
| `autocomplete.option.selected.background` | `--p-autocomplete-option-selected-background` | Selected background of option |
| `autocomplete.option.selected.focus.background` | `--p-autocomplete-option-selected-focus-background` | Selected focus background of option |
| `autocomplete.option.color` | `--p-autocomplete-option-color` | Color of option |
| `autocomplete.option.focus.color` | `--p-autocomplete-option-focus-color` | Focus color of option |
| `autocomplete.option.selected.color` | `--p-autocomplete-option-selected-color` | Selected color of option |
| `autocomplete.option.selected.focus.color` | `--p-autocomplete-option-selected-focus-color` | Selected focus color of option |
| `autocomplete.option.padding` | `--p-autocomplete-option-padding` | Padding of option |
| `autocomplete.option.border.radius` | `--p-autocomplete-option-border-radius` | Border radius of option |
| `autocomplete.option.group.background` | `--p-autocomplete-option-group-background` | Background of option group |
| `autocomplete.option.group.color` | `--p-autocomplete-option-group-color` | Color of option group |
| `autocomplete.option.group.font.weight` | `--p-autocomplete-option-group-font-weight` | Font weight of option group |
| `autocomplete.option.group.padding` | `--p-autocomplete-option-group-padding` | Padding of option group |
| `autocomplete.dropdown.width` | `--p-autocomplete-dropdown-width` | Width of dropdown |
| `autocomplete.dropdown.sm.width` | `--p-autocomplete-dropdown-sm-width` | Sm width of dropdown |
| `autocomplete.dropdown.lg.width` | `--p-autocomplete-dropdown-lg-width` | Lg width of dropdown |
| `autocomplete.dropdown.border.color` | `--p-autocomplete-dropdown-border-color` | Border color of dropdown |
| `autocomplete.dropdown.hover.border.color` | `--p-autocomplete-dropdown-hover-border-color` | Hover border color of dropdown |
| `autocomplete.dropdown.active.border.color` | `--p-autocomplete-dropdown-active-border-color` | Active border color of dropdown |
| `autocomplete.dropdown.border.radius` | `--p-autocomplete-dropdown-border-radius` | Border radius of dropdown |
| `autocomplete.dropdown.focus.ring.width` | `--p-autocomplete-dropdown-focus-ring-width` | Focus ring width of dropdown |
| `autocomplete.dropdown.focus.ring.style` | `--p-autocomplete-dropdown-focus-ring-style` | Focus ring style of dropdown |
| `autocomplete.dropdown.focus.ring.color` | `--p-autocomplete-dropdown-focus-ring-color` | Focus ring color of dropdown |
| `autocomplete.dropdown.focus.ring.offset` | `--p-autocomplete-dropdown-focus-ring-offset` | Focus ring offset of dropdown |
| `autocomplete.dropdown.focus.ring.shadow` | `--p-autocomplete-dropdown-focus-ring-shadow` | Focus ring shadow of dropdown |
| `autocomplete.dropdown.background` | `--p-autocomplete-dropdown-background` | Background of dropdown |
| `autocomplete.dropdown.hover.background` | `--p-autocomplete-dropdown-hover-background` | Hover background of dropdown |
| `autocomplete.dropdown.active.background` | `--p-autocomplete-dropdown-active-background` | Active background of dropdown |
| `autocomplete.dropdown.color` | `--p-autocomplete-dropdown-color` | Color of dropdown |
| `autocomplete.dropdown.hover.color` | `--p-autocomplete-dropdown-hover-color` | Hover color of dropdown |
| `autocomplete.dropdown.active.color` | `--p-autocomplete-dropdown-active-color` | Active color of dropdown |
| `autocomplete.chip.border.radius` | `--p-autocomplete-chip-border-radius` | Border radius of chip |
| `autocomplete.chip.focus.background` | `--p-autocomplete-chip-focus-background` | Focus background of chip |
| `autocomplete.chip.focus.color` | `--p-autocomplete-chip-focus-color` | Focus color of chip |
| `autocomplete.empty.message.padding` | `--p-autocomplete-empty-message-padding` | Padding of empty message |
