# OrderList

OrderList is used to sort a collection.

## Import

```typescript
import { OrderListModule, OrderList } from '@openng/optimus-ui/orderlist';
```

## Basic

OrderList is used as a controlled input with value property. Content of a list item needs to be defined with the item template that receives an object in the list as parameter.

```typescript
import { Component, OnInit, inject, signal } from '@angular/core';
import { OrderListModule } from '@openng/optimus-ui/orderlist';
import { ProductService } from '@/service/productservice';
import { Product } from '@/domain/product';

@Component({
    template: `
        <div class="card sm:flex sm:justify-center">
            <p-orderlist [value]="products()" dataKey="id" [responsive]="true" breakpoint="575px">
                <ng-template #item let-option>
                    {{ option.name }}
                </ng-template>
            </p-orderlist>
        </div>
    `,
    standalone: true,
    imports: [OrderListModule],
    providers: [ProductService]
})
export class OrderlistBasicDemo implements OnInit {
    private productService = inject(ProductService);
    products = signal<Product[]>([]);

    ngOnInit() {
        this.productService.getProductsSmall().then((cars) => {
            this.products.set(cars);
        });
    }

    getSeverity(status: string) {
        switch (status) {
            case 'INSTOCK':
                return 'success';
            case 'LOWSTOCK':
                return 'warning';
            case 'OUTOFSTOCK':
                return 'danger';
        }
    }
}
```

## Filter

Filter value is checked against the property of an object configured with the filterBy property

```typescript
import { Component, OnInit, inject, signal } from '@angular/core';
import { OrderListModule } from '@openng/optimus-ui/orderlist';
import { ProductService } from '@/service/productservice';
import { Product } from '@/domain/product';

@Component({
    template: `
        <div class="card sm:flex sm:justify-center">
            <p-orderlist [value]="products()" filterBy="name" filterPlaceholder="Filter by name" [responsive]="true" breakpoint="575px" scrollHeight="20rem" class="sm:min-w-96">
                <ng-template let-option let-selected="selected" #item>
                    <div class="flex flex-wrap p-1 items-center gap-4 w-full">
                        <img class="w-12 shrink-0 rounded" src="https://optimus.openng.org/demo/product/{{ option.image }}" [alt]="option.name" />
                        <div class="flex-1 flex flex-col">
                            <span class="font-medium text-sm">{{ option.name }}</span>
                            <span
                                [ngClass]="{
                                    'text-sm': true,
                                    'text-surface-500': !selected,
                                    'dark:text-surface-400': !selected,
                                    'text-inherit': selected
                                }"
                                >{{ option.category }}</span
                            >
                        </div>
                        <span class="font-bold sm:ml-8">{{ '$' + option.price }}</span>
                    </div>
                </ng-template>
            </p-orderlist>
        </div>
    `,
    standalone: true,
    imports: [OrderListModule],
    providers: [ProductService]
})
export class OrderlistFilterDemo implements OnInit {
    private productService = inject(ProductService);
    products = signal<Product[]>([]);

    ngOnInit() {
        this.productService.getProductsSmall().then((cars) => {
            this.products.set(cars);
        });
    }

    getSeverity(status: string) {
        switch (status) {
            case 'INSTOCK':
                return 'success';
            case 'LOWSTOCK':
                return 'warning';
            case 'OUTOFSTOCK':
                return 'danger';
        }
    }
}
```

## Template

For custom content support define an item template that gets the item instance as a parameter. In addition header template is provided for further customization.

```typescript
import { Component, OnInit, inject, signal } from '@angular/core';
import { OrderListModule } from '@openng/optimus-ui/orderlist';
import { ProductService } from '@/service/productservice';
import { Product } from '@/domain/product';

@Component({
    template: `
        <div class="card sm:flex sm:justify-center">
            <p-orderlist [value]="products()" dataKey="id" [responsive]="true" breakpoint="575px" scrollHeight="20rem">
                <ng-template let-option let-selected="selected" #item>
                    <div class="flex flex-wrap p-1 items-center gap-4 w-full">
                        <img class="w-12 shrink-0 rounded" src="https://optimus.openng.org/demo/product/{{ option.image }}" [alt]="option.name" />
                        <div class="flex-1 flex flex-col">
                            <span class="font-medium text-sm">{{ option.name }}</span>
                            <span
                                [ngClass]="{
                                    'text-sm': true,
                                    'text-surface-500': !selected,
                                    'dark:text-surface-400': !selected,
                                    'text-inherit': selected
                                }"
                                >{{ option.category }}</span
                            >
                        </div>
                        <span class="font-bold sm:ml-8">{{ '$' + option.price }}</span>
                    </div>
                </ng-template>
            </p-orderlist>
        </div>
    `,
    standalone: true,
    imports: [OrderListModule],
    providers: [ProductService]
})
export class OrderlistTemplateDemo implements OnInit {
    private productService = inject(ProductService);
    products = signal<Product[]>([]);

    ngOnInit() {
        this.productService.getProductsSmall().then((cars) => {
            this.products.set(cars);
        });
    }

    getSeverity(status: string) {
        switch (status) {
            case 'INSTOCK':
                return 'success';
            case 'LOWSTOCK':
                return 'warning';
            case 'OUTOFSTOCK':
                return 'danger';
        }
    }
}
```

## Accessibility

Screen Reader Value to describe the source listbox and target listbox can be provided with sourceListProps and targetListProps by passing aria-labelledby or aria-label props. The list elements has a listbox role with the aria-multiselectable attribute. Each list item has an option role with aria-selected and aria-disabled as their attributes. Controls buttons are button elements with an aria-label that refers to the aria.moveTop , aria.moveUp , aria.moveDown , aria.moveBottom , aria.moveTo , aria.moveAllTo , aria.moveFrom and aria.moveAllFrom properties of the locale API by default, alternatively you may use moveTopButtonProps , moveUpButtonProps , moveDownButtonProps , moveToButtonProps , moveAllToButtonProps , moveFromButtonProps , moveFromButtonProps and moveAllFromButtonProps to customize the buttons like overriding the default aria-label attributes. OrderList Keyboard Support Key Function tab Moves focus to the first selected option, if there is none then first option receives the focus. up arrow Moves focus to the previous option. down arrow Moves focus to the next option. enter Toggles the selected state of the focused option. space Toggles the selected state of the focused option. home Moves focus to the first option. end Moves focus to the last option. shift + down arrow Moves focus to the next option and toggles the selection state. shift + up arrow Moves focus to the previous option and toggles the selection state. shift + space Selects the items between the most recently selected option and the focused option. control + shift + home Selects the focused options and all the options up to the first one. control + shift + end Selects the focused options and all the options down to the first one. control + a Selects all options. Buttons Keyboard Support Key Function enter Executes button action. space Executes button action.

## Dragdrop

Items can be reordered using drag and drop by enabling dragdrop property. Depends on &#64;angular/cdk package.

```typescript
import { Component, OnInit, inject, signal } from '@angular/core';
import { OrderListModule } from '@openng/optimus-ui/orderlist';
import { ProductService } from '@/service/productservice';
import { Product } from '@/domain/product';

@Component({
    template: `
        <div class="card sm:flex sm:justify-center">
            <p-orderlist [value]="products()" dataKey="id" [dragdrop]="true" [responsive]="true" breakpoint="575px" scrollHeight="20rem">
                <ng-template let-option let-selected="selected" #item>
                    <div class="flex flex-wrap p-1 items-center gap-4 w-full">
                        <img class="w-12 shrink-0 rounded" src="https://optimus.openng.org/demo/product/{{ option.image }}" [alt]="option.name" />
                        <div class="flex-1 flex flex-col">
                            <span class="font-medium text-sm">{{ option.name }}</span>
                            <span
                                [ngClass]="{
                                    'text-sm': true,
                                    'text-surface-500': !selected,
                                    'dark:text-surface-400': !selected,
                                    'text-inherit': selected
                                }"
                                >{{ option.category }}</span
                            >
                        </div>
                        <span class="font-bold sm:ml-8">{{ '$' + option.price }}</span>
                    </div>
                </ng-template>
            </p-orderlist>
        </div>
    `,
    standalone: true,
    imports: [OrderListModule],
    providers: [ProductService]
})
export class OrderlistDragdropDemo implements OnInit {
    private productService = inject(ProductService);
    products = signal<Product[]>([]);

    ngOnInit() {
        this.productService.getProductsSmall().then((cars) => {
            this.products.set(cars);
        });
    }

    getSeverity(status: string) {
        switch (status) {
            case 'INSTOCK':
                return 'success';
            case 'LOWSTOCK':
                return 'warning';
            case 'OUTOFSTOCK':
                return 'danger';
        }
    }
}
```

## API

### OrderList

Selector: `p-orderList, p-orderlist, p-order-list` · Component, standalone

OrderList is used to manage the order of a collection.

#### Inputs

| Name | Type | Default | Description |
|---|---|---|---|
| `ariaFilterLabel` | `string \| undefined` | - | Defines a string that labels the filter input. |
| `ariaLabel` | `string \| undefined` | - | Defines a string that labels the input for accessibility. |
| `ariaLabelledBy` | `string \| undefined` | - | Specifies one or more IDs in the DOM that labels the input field. |
| `autoOptionFocus` | `boolean` | `true` | Whether to focus on the first visible or selected element. |
| `breakpoint` | `string` | `'960px'` | Indicates the width of the screen at which the component should change its behavior. |
| `buttonProps` | `ButtonProps` | `{ severity: 'secondary' }` | Used to pass all properties of the ButtonProps to the Button component. |
| `controlsPosition` | `"left" \| "right"` | `'left'` | Defines the location of the buttons with respect to the list. |
| `dataKey` | `string \| undefined` | - | Name of the field that uniquely identifies the record in the data. |
| `disabled` | `boolean` | - | When present, it specifies that the component should be disabled. |
| `dragdrop` | `boolean` | `false` | Whether to enable dragdrop based reordering. |
| `dt` | `Object \| undefined` | - | Defines scoped design tokens of the component. |
| `filterBy` | `string \| undefined` | - | When specified displays an input field to filter the items on keyup and decides which fields to search against. |
| `filterLocale` | `string \| undefined` | - | Locale to use in filtering. The default locale is the host environment's current locale. |
| `filterMatchMode` | `"in" \| "contains" \| "startsWith" \| "endsWith" \| "equals" \| "notEquals" \| "lt" \| "lte" \| "gt" \| "gte"` | `'contains'` | Defines how the items are filtered. |
| `filterPlaceholder` | `string \| undefined` | - | Placeholder of the filter input. |
| `header` | `string \| undefined` | - | Text for the caption. |
| `listStyle` | `{ [klass: string]: any; } \| null \| undefined` | - | Inline style of the list element. |
| `metaKeySelection` | `boolean` | `false` | When true metaKey needs to be pressed to select or unselect an item and when set to false selection of each item can be toggled individually. On touch enabled devices, metaKeySelection is turned off automatically. |
| `moveBottomButtonProps` | `ButtonProps` | - | Used to pass all properties of the ButtonProps to the move bottom button inside the component. |
| `moveDownButtonProps` | `ButtonProps` | - | Used to pass all properties of the ButtonProps to the move down button inside the component. |
| `moveTopButtonProps` | `ButtonProps` | - | Used to pass all properties of the ButtonProps to the move top button inside the component. |
| `moveUpButtonProps` | `ButtonProps` | - | Used to pass all properties of the ButtonProps to the move up button inside the component. |
| `pt` | `OrderListPassThrough` | - | Used to pass attributes to DOM elements inside the component. |
| `ptOptions` | `PassThroughOptions \| undefined` | - | Used to configure passthrough(pt) options of the component. |
| `responsive` | `boolean \| undefined` | - | A boolean value that indicates whether the component should be responsive. |
| `scrollHeight` | `string` | `'14rem'` | Height of the viewport, a scrollbar is defined if height of list exceeds this value. |
| `selection` | `any[]` | - | A list of values that are currently selected. Two-way: `[(selection)]`. |
| `stripedRows` | `boolean \| undefined` | - | Whether to displays rows with alternating colors. |
| `styleClass` | `string \| undefined` | - | Style class of the component. **Deprecated**: since v20.0.0, use `class` instead. |
| `tabindex` | `number \| undefined` | - | Index of the element in tabbing order. |
| `trackBy` | `Function` | `(index: number, item: any) => item` | Function to optimize the dom operations by delegating to ngForTrackBy, default algorithm checks for object identity. |
| `unstyled` | `boolean \| undefined` | - | Indicates whether the component should be rendered without styles. |
| `value` | `any[] \| undefined` | - | Array of values to be displayed in the component. It represents the data source for the list of items. |

#### Outputs

| Name | Payload | Description |
|---|---|---|
| `onBlur` | `Event` | Callback to invoke when the list is blurred |
| `onFilterEvent` | `OrderListFilterEvent` | Callback to invoke when filtering occurs. |
| `onFocus` | `Event` | Callback to invoke when the list is focused |
| `onReorder` | `any` | Callback to invoke when list is reordered. |
| `onSelectionChange` | `OrderListSelectionChangeEvent` | Callback to invoke when selection changes. |
| `selectionChange` | `any` | Callback to invoke on selection change. |

#### Templates

Define with `<ng-template #name let-context>` inside the `p-orderList` element.

| Name | Context | Description |
|---|---|---|
| `item` | `OrderListItemTemplateContext` | Custom item template. |
| `emptymessage` | - | Custom empty template. |
| `emptyfiltermessage` | - | Custom empty filter template. |
| `filter` | `OrderListFilterTemplateContext` | Custom filter template. |
| `header` | - | Custom header template. |
| `moveupicon` | - | Custom move up icon template. |
| `movetopicon` | - | Custom move top icon template. |
| `movedownicon` | - | Custom move down icon template. |
| `movebottomicon` | - | Custom move bottom icon template. |
| `filtericon` | - | Custom filter icon template. |

#### Methods

| Name | Parameters | Returns | Description |
|---|---|---|---|
| `resetFilter` | - | `void` | Callback to invoke on filter reset. |

## Pass Through Options

| Name | Type | Description |
|---|---|---|
| `host` | `PassThroughOption<HTMLElement, I>` | Used to pass attributes to the host's DOM element. |
| `root` | `PassThroughOption<HTMLDivElement, I>` | Used to pass attributes to the root's DOM element. |
| `controls` | `PassThroughOption<HTMLDivElement, I>` | Used to pass attributes to the controls container's DOM element. |
| `pcMoveUpButton` | `ButtonPassThrough` | Used to pass attributes to the move up button's DOM element. |
| `pcMoveTopButton` | `ButtonPassThrough` | Used to pass attributes to the move top button's DOM element. |
| `pcMoveDownButton` | `ButtonPassThrough` | Used to pass attributes to the move down button's DOM element. |
| `pcMoveBottomButton` | `ButtonPassThrough` | Used to pass attributes to the move bottom button's DOM element. |
| `pcListbox` | `ListBoxPassThrough` | Used to pass attributes to the Listbox component. |

## Theming

### CSS Classes

| Class | Description |
|---|---|
| `p-orderlist` | Class name of the root element |
| `p-orderlist-controls` | Class name of the controls element |

### Design Tokens

| Token | CSS Variable | Description |
|---|---|---|
| `orderlist.gap` | `--p-orderlist-gap` | Gap of root |
| `orderlist.controls.gap` | `--p-orderlist-controls-gap` | Gap of controls |
