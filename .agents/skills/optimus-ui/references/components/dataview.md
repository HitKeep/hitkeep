# DataView

DataView displays data in grid grid-cols-12 gap-4 or list layout with pagination and sorting features.

## Import

```typescript
import { DataViewModule, DataView } from '@openng/optimus-ui/dataview';
```

## Basic

DataView requires a value to display along with a list template that receives an object in the collection to return content.

```typescript
import { Component, OnInit, inject, signal } from '@angular/core';
import { ButtonModule } from '@openng/optimus-ui/button';
import { DataViewModule } from '@openng/optimus-ui/dataview';
import { TagModule } from '@openng/optimus-ui/tag';
import { ProductService } from '@/service/productservice';
import { Product } from '@/domain/product';

@Component({
    template: `
        <div class="card">
            <p-dataview #dv [value]="products()">
                <ng-template #list let-items>
                    <div class="grid grid-cols-12 gap-4 grid-nogutter">
                        <div class="col-span-12" *ngFor="let item of items; let first = first">
                            <div class="flex flex-col sm:flex-row sm:items-center p-6 gap-4" [ngClass]="{ 'border-t border-surface-200 dark:border-surface-700': !first }">
                                <div class="md:w-40 relative">
                                    <img class="block xl:block mx-auto rounded-border w-full" [src]="'https://optimus.openng.org/demo/product/' + item.image" [alt]="item.name" />
                                    <p-tag [value]="item.inventoryStatus" [severity]="getSeverity(item)" class="absolute dark:!bg-surface-900" [style.left.px]="4" [style.top.px]="4" />
                                </div>
                                <div class="flex flex-col md:flex-row justify-between md:items-center flex-1 gap-6">
                                    <div class="flex flex-row md:flex-col justify-between items-start gap-2">
                                        <div>
                                            <span class="font-medium text-secondary text-sm">{{ item.category }}</span>
                                            <div class="text-lg font-medium text-surface-900 dark:text-surface-0 mt-2">{{ item.name }}</div>
                                        </div>
                                        <div class="bg-surface-100 dark:bg-surface-700 p-1" style="border-radius: 30px">
                                            <div
                                                class="bg-surface-0 dark:bg-surface-900 flex items-center gap-2 justify-center py-1 px-2"
                                                style="border-radius: 30px; box-shadow: 0px 1px 2px 0px rgba(0, 0, 0, 0.04), 0px 1px 2px 0px rgba(0, 0, 0, 0.06)"
                                            >
                                                <span class="text-surface-900 dark:text-surface-0 font-medium text-sm">{{ item.rating }}</span>
                                                <i class="pi pi-star-fill text-yellow-500"></i>
                                            </div>
                                        </div>
                                    </div>
                                    <div class="flex flex-col md:items-end gap-8">
                                        <span class="text-xl font-semibold text-surface-900 dark:text-surface-0">{{ '$' + item.price }}</span>
                                        <div class="flex flex-row-reverse md:flex-row gap-2">
                                            <p-button icon="pi pi-heart" [outlined]="true" />
                                            <p-button icon="pi pi-shopping-cart" class="flex-auto md:flex-initial whitespace-nowrap" label="Buy Now" [disabled]="item.inventoryStatus === 'OUTOFSTOCK'" />
                                        </div>
                                    </div>
                                </div>
                            </div>
                        </div>
                    </div>
                </ng-template>
            </p-dataview>
        </div>
    `,
    standalone: true,
    imports: [ButtonModule, DataViewModule, TagModule],
    providers: [ProductService]
})
export class DataviewBasicDemo implements OnInit {
    private productService = inject(ProductService);
    products = signal<any>([]);
    productService = inject(ProductService);

    ngOnInit() {
        this.productService.getProducts().then((data) => {
            const d = data.slice(0, 5);
            this.products.set([...d]);
        });
    }

    getSeverity(product: Product) {
        switch (product.inventoryStatus) {
            case 'INSTOCK':
                return 'success';
        
            case 'LOWSTOCK':
                return 'warn';
        
            case 'OUTOFSTOCK':
                return 'danger';
        
            default:
                return null;
        }
    }
}
```

## Pagination

Pagination is enabled with the paginator and rows properties. Refer to the Paginator for more information about customizing the paginator.

```typescript
import { Component, OnInit, inject, signal } from '@angular/core';
import { ButtonModule } from '@openng/optimus-ui/button';
import { DataViewModule } from '@openng/optimus-ui/dataview';
import { TagModule } from '@openng/optimus-ui/tag';
import { ProductService } from '@/service/productservice';
import { Product } from '@/domain/product';

@Component({
    template: `
        <div class="card">
            <p-dataview #dv [value]="products()" [rows]="5" [paginator]="true">
                <ng-template #list let-items>
                    <div class="grid grid-cols-12 gap-4 grid-nogutter">
                        <div class="col-span-12" *ngFor="let item of items; let first = first">
                            <div class="flex flex-col sm:flex-row sm:items-center p-6 gap-4" [ngClass]="{ 'border-t border-surface-200 dark:border-surface-700': !first }">
                                <div class="md:w-40 relative">
                                    <img class="block xl:block mx-auto rounded-border w-full" [src]="'https://optimus.openng.org/demo/product/' + item.image" [alt]="item.name" />
                                    <p-tag [value]="item.inventoryStatus" [severity]="getSeverity(item)" class="absolute dark:!bg-surface-900" [style.left.px]="4" [style.top.px]="4" />
                                </div>
                                <div class="flex flex-col md:flex-row justify-between md:items-center flex-1 gap-6">
                                    <div class="flex flex-row md:flex-col justify-between items-start gap-2">
                                        <div>
                                            <span class="font-medium text-secondary text-sm">{{ item.category }}</span>
                                            <div class="text-lg font-medium text-surface-900 dark:text-surface-0 mt-2">{{ item.name }}</div>
                                        </div>
                                        <div class="bg-surface-100 dark:bg-surface-700 p-1" style="border-radius: 30px">
                                            <div
                                                class="bg-surface-0 dark:bg-surface-900 flex items-center gap-2 justify-center py-1 px-2"
                                                style="border-radius: 30px; box-shadow: 0px 1px 2px 0px rgba(0, 0, 0, 0.04), 0px 1px 2px 0px rgba(0, 0, 0, 0.06)"
                                            >
                                                <span class="text-surface-900 dark:text-surface-0 font-medium text-sm">{{ item.rating }}</span>
                                                <i class="pi pi-star-fill text-yellow-500"></i>
                                            </div>
                                        </div>
                                    </div>
                                    <div class="flex flex-col md:items-end gap-8">
                                        <span class="text-xl font-semibold text-surface-900 dark:text-surface-0">{{ '$' + item.price }}</span>
                                        <div class="flex flex-row-reverse md:flex-row gap-2">
                                            <p-button icon="pi pi-heart" [outlined]="true" />
                                            <p-button icon="pi pi-shopping-cart" class="flex-auto md:flex-initial whitespace-nowrap" label="Buy Now" [disabled]="item.inventoryStatus === 'OUTOFSTOCK'" />
                                        </div>
                                    </div>
                                </div>
                            </div>
                        </div>
                    </div>
                </ng-template>
            </p-dataview>
        </div>
    `,
    standalone: true,
    imports: [ButtonModule, DataViewModule, TagModule],
    providers: [ProductService]
})
export class DataviewPaginationDemo implements OnInit {
    private productService = inject(ProductService);
    products = signal<any>([]);

    ngOnInit() {
        this.productService.getProducts().then((data) => {
            this.products.set(data);
        });
    }
}
```

## Sorting

Built-in sorting is controlled by bindings sortField and sortOrder properties from a custom UI.

```typescript
import { Component, OnInit, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { ButtonModule } from '@openng/optimus-ui/button';
import { DataViewModule } from '@openng/optimus-ui/dataview';
import { SelectModule } from '@openng/optimus-ui/select';
import { TagModule } from '@openng/optimus-ui/tag';
import { ProductService } from '@/service/productservice';
import { SelectItem } from '@openng/optimus-ui/api';
import { Product } from '@/domain/product';

@Component({
    template: `
        <div class="card">
            <p-dataview #dv [value]="products()" [sortField]="sortField" [sortOrder]="sortOrder">
                <ng-template #header>
                    <div class="flex flex-col md:flex-row md:justify-between">
                        <p-select [options]="sortOptions" [(ngModel)]="sortKey" placeholder="Sort By Price" (onChange)="onSortChange($event)" class="mb-2 md:mb-0" />
                    </div>
                </ng-template>
                <ng-template #list let-items>
                    <div class="grid grid-cols-12 gap-4 grid-nogutter">
                        <div class="col-span-12" *ngFor="let item of items; let first = first">
                            <div class="flex flex-col sm:flex-row sm:items-center p-6 gap-4" [ngClass]="{ 'border-t border-surface-200 dark:border-surface-700': !first }">
                                <div class="md:w-40 relative">
                                    <img class="block xl:block mx-auto rounded-border w-full" [src]="'https://optimus.openng.org/demo/product/' + item.image" [alt]="item.name" />
                                    <p-tag [value]="item.inventoryStatus" [severity]="getSeverity(item)" class="dark:!bg-surface-900 absolute" [style.left.px]="4" [style.top.px]="4" />
                                </div>
                                <div class="flex flex-col md:flex-row justify-between md:items-center flex-1 gap-6">
                                    <div class="flex flex-row md:flex-col justify-between items-start gap-2">
                                        <div>
                                            <span class="font-medium text-secondary text-sm">{{ item.category }}</span>
                                            <div class="text-lg font-medium text-surface-900 dark:text-surface-0 mt-2">{{ item.name }}</div>
                                        </div>
                                        <div class="bg-surface-100 dark:bg-surface-700 p-1" style="border-radius: 30px">
                                            <div
                                                class="bg-surface-0 dark:bg-surface-900 flex items-center gap-2 justify-center py-1 px-2"
                                                style="border-radius: 30px; box-shadow: 0px 1px 2px 0px rgba(0, 0, 0, 0.04), 0px 1px 2px 0px rgba(0, 0, 0, 0.06)"
                                            >
                                                <span class="text-surface-900 dark:text-surface-0 font-medium text-sm">{{ item.rating }}</span>
                                                <i class="pi pi-star-fill text-yellow-500"></i>
                                            </div>
                                        </div>
                                    </div>
                                    <div class="flex flex-col md:items-end gap-8">
                                        <span class="text-xl font-semibold text-surface-900 dark:text-surface-0">{{ '$' + item.price }}</span>
                                        <div class="flex flex-row-reverse md:flex-row gap-2">
                                            <p-button icon="pi pi-heart" [outlined]="true" />
                                            <p-button icon="pi pi-shopping-cart" class="flex-auto md:flex-initial whitespace-nowrap" label="Buy Now" [disabled]="item.inventoryStatus === 'OUTOFSTOCK'" />
                                        </div>
                                    </div>
                                </div>
                            </div>
                        </div>
                    </div>
                </ng-template>
            </p-dataview>
        </div>
    `,
    standalone: true,
    imports: [ButtonModule, DataViewModule, SelectModule, TagModule, FormsModule],
    providers: [ProductService]
})
export class DataviewSortingDemo implements OnInit {
    private productService = inject(ProductService);
    sortOptions!: SelectItem[];
    sortOrder!: number;
    sortField!: string;
    products = signal<any>([]);

    ngOnInit() {
        this.productService.getProducts().then((data) => this.products.set(data.slice(0, 5)));
        this.sortOptions = [
            { label: 'Price High to Low', value: '!price' },
            { label: 'Price Low to High', value: 'price' }
        ];
    }

    getSeverity(product: Product) {
        switch (product.inventoryStatus) {
            case 'INSTOCK':
                return 'success';
        
            case 'LOWSTOCK':
                return 'warn';
        
            case 'OUTOFSTOCK':
                return 'danger';
        
            default:
                return null;
        }
    }
}
```

## Layout

DataView supports list and grid display modes defined with the layout property. The grid mode is not built-in for flexibility purposes and requires a library with CSS grid features like Tailwind.

```typescript
import { Component, OnInit, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { DataViewModule } from '@openng/optimus-ui/dataview';
import { SelectButtonModule } from '@openng/optimus-ui/selectbutton';
import { TagModule } from '@openng/optimus-ui/tag';
import { ButtonModule } from '@openng/optimus-ui/button';
import { ProductService } from '@/service/productservice';
import { Product } from '@/domain/product';

@Component({
    template: `
        <div class="card">
            <p-dataview #dv [value]="products()" [layout]="layout">
                <ng-template #header>
                    <div class="flex justify-end">
                        <p-selectbutton [(ngModel)]="layout" [options]="options" [allowEmpty]="false">
                            <ng-template #item let-item>
                                <i class="pi " [ngClass]="{ 'pi-bars': item === 'list', 'pi-table': item === 'grid' }"></i>
                            </ng-template>
                        </p-selectbutton>
                    </div>
                </ng-template>
                <ng-template #list let-items>
                    <div *ngFor="let item of items; let first = first">
                        <div class="flex flex-col sm:flex-row sm:items-center p-6 gap-4" [ngClass]="{ 'border-t border-surface-200 dark:border-surface-700': !first }">
                            <div class="md:w-40 relative">
                                <img class="block xl:block mx-auto rounded w-full" [src]="'https://optimus.openng.org/demo/product/' + item.image" [alt]="item.name" />
                                <p-tag [value]="item.inventoryStatus" [severity]="getSeverity(item)" class="absolute dark:!bg-surface-900" [style.left.px]="4" [style.top.px]="4" />
                            </div>
                            <div class="flex flex-col md:flex-row justify-between md:items-center flex-1 gap-6">
                                <div class="flex flex-row md:flex-col justify-between items-start gap-2">
                                    <div>
                                        <span class="font-medium text-surface-500 dark:text-surface-400 text-sm">{{ item.category }}</span>
                                        <div class="text-lg font-medium mt-2">{{ item.name }}</div>
                                    </div>
                                    <div class="bg-surface-100 p-1" style="border-radius: 30px">
                                        <div class="bg-surface-0 flex items-center gap-2 justify-center py-1 px-2" style="border-radius: 30px; box-shadow: 0px 1px 2px 0px rgba(0, 0, 0, 0.04), 0px 1px 2px 0px rgba(0, 0, 0, 0.06)">
                                            <span class="text-surface-900 font-medium text-sm">{{ item.rating }}</span>
                                            <i class="pi pi-star-fill text-yellow-500"></i>
                                        </div>
                                    </div>
                                </div>
                                <div class="flex flex-col md:items-end gap-8">
                                    <span class="text-xl font-semibold">{{ item.price | currency: 'USD' }}</span>
                                    <div class="flex flex-row-reverse md:flex-row gap-2">
                                        <button pButton icon="pi pi-heart" [outlined]="true"></button>
                                        <button pButton icon="pi pi-shopping-cart" label="Buy Now" [disabled]="item.inventoryStatus === 'OUTOFSTOCK'" class="flex-auto md:flex-initial whitespace-nowrap"></button>
                                    </div>
                                </div>
                            </div>
                        </div>
                    </div>
                </ng-template>
                <ng-template #grid let-items>
                    <div class="grid grid-cols-12 gap-4">
                        <div *ngFor="let product of items" class="col-span-12 sm:col-span-6 md:col-span-4 xl:col-span-6 p-2">
                            <div class="p-6 border border-surface-200 dark:border-surface-700 bg-surface-0 dark:bg-surface-900 rounded flex flex-col">
                                <div class="bg-surface-50 flex justify-center rounded p-4">
                                    <div class="relative mx-auto">
                                        <img class="rounded w-full" [src]="'https://optimus.openng.org/demo/product/' + product.image" [alt]="product.name" style="max-width: 300px" />
                                        <p-tag [value]="product.inventoryStatus" [severity]="getSeverity(product)" class="absolute dark:!bg-surface-900" [style.left.px]="4" [style.top.px]="4" />
                                    </div>
                                </div>
                                <div class="pt-6">
                                    <div class="flex flex-row justify-between products-start gap-2">
                                        <div>
                                            <span class="font-medium text-surface-500 dark:text-surface-400 text-sm">{{ product.category }}</span>
                                            <div class="text-lg font-medium mt-1">{{ product.name }}</div>
                                        </div>
                                        <div class="bg-surface-100 p-1" style="border-radius: 30px; height:100%">
                                            <div class="bg-surface-0 flex products-center gap-2 justify-center py-1 px-2" style="border-radius: 30px; box-shadow: 0px 1px 2px 0px rgba(0, 0, 0, 0.04), 0px 1px 2px 0px rgba(0, 0, 0, 0.06)">
                                                <span class="text-surface-900 font-medium text-sm">{{ product.rating }}</span>
                                                <i class="pi pi-star-fill text-yellow-500"></i>
                                            </div>
                                        </div>
                                    </div>
                                    <div class="flex flex-col gap-6 mt-6">
                                        <span class="text-2xl font-semibold">{{ product.price | currency: 'USD' }}</span>
                                        <div class="flex gap-2">
                                            <button pButton icon="pi pi-shopping-cart" label="Buy Now" [disabled]="product.inventoryStatus === 'OUTOFSTOCK'" class="flex-auto whitespace-nowrap"></button>
                                            <button pButton icon="pi pi-heart" outlined></button>
                                        </div>
                                    </div>
                                </div>
                            </div>
                        </div>
                    </div>
                </ng-template>
            </p-dataview>
        </div>
    `,
    standalone: true,
    imports: [DataViewModule, SelectButtonModule, TagModule, ButtonModule, FormsModule],
    providers: [ProductService]
})
export class DataviewLayoutDemo implements OnInit {
    private productService = inject(ProductService);
    products = signal<any>([]);
    options: any[] = ['list', 'grid'];

    ngOnInit() {
        this.productService.getProducts().then((data) => {
            this.products.set([...data.slice(0, 12)]);
        });
    }
}
```

## Loading

While data is being loaded. Skeleton component may be used to indicate the busy state.

```typescript
import { Component, OnInit, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { DataViewModule } from '@openng/optimus-ui/dataview';
import { SelectButtonModule } from '@openng/optimus-ui/selectbutton';
import { SkeletonModule } from '@openng/optimus-ui/skeleton';
import { ProductService } from '@/service/productservice';
import { Product } from '@/domain/product';

@Component({
    template: `
        <div class="card">
            <p-dataview #dv [value]="products()" [layout]="layout">
                <ng-template #header>
                    <div class="flex justify-end">
                        <p-selectbutton [(ngModel)]="layout" [options]="options" [allowEmpty]="false">
                            <ng-template #item let-option>
                                <i [class]="option === 'list' ? 'pi pi-bars' : 'pi pi-table'"></i>
                            </ng-template>
                        </p-selectbutton>
                    </div>
                </ng-template>
                <ng-template #list let-items>
                    <div class="flex flex-col">
                        <div *ngFor="let i of counterArray(6); let first = first">
                            <div class="flex flex-col xl:flex-row xl:items-start p-6 gap-6" [ngClass]="{ 'border-t border-surface-200 dark:border-surface-700': !first }">
                                <p-skeleton class="!w-9/12 sm:!w-64 xl:!w-40 !h-24 mx-auto" />
                                <div class="flex flex-col sm:flex-row justify-between items-center xl:items-start flex-1 gap-6">
                                    <div class="flex flex-col items-center sm:items-start gap-4">
                                        <p-skeleton width="8rem" height="2rem" />
                                        <p-skeleton width="6rem" height="1rem" />
                                        <div class="flex items-center gap-4">
                                            <p-skeleton width="6rem" height="1rem" />
                                            <p-skeleton width="3rem" height="1rem" />
                                        </div>
                                    </div>
                                    <div class="flex sm:flex-col items-center sm:items-end gap-4 sm:gap-2">
                                        <p-skeleton width="4rem" height="2rem" />
                                        <p-skeleton size="3rem" shape="circle" />
                                    </div>
                                </div>
                            </div>
                        </div>
                    </div>
                </ng-template>
                <ng-template #grid let-items>
                    <div class="grid grid-cols-12 gap-4">
                        <div *ngFor="let i of counterArray(6); let first = first" class="col-span-12 sm:col-span-6 xl:col-span-4 p-2">
                            <div class="p-6 border border-surface-200 dark:border-surface-700 bg-surface-0 dark:bg-surface-900 rounded">
                                <div class="flex flex-wrap items-center justify-between gap-2">
                                    <p-skeleton width="6rem" height="2rem" />
                                    <p-skeleton width="3rem" height="1rem" />
                                </div>
                                <div class="flex flex-col items-center gap-4 py-8">
                                    <p-skeleton height="10rem" class="w-3/4" class="w-3/4" />
                                    <p-skeleton width="8rem" height="2rem" />
                                    <p-skeleton width="6rem" height="1rem" />
                                </div>
                                <div class="flex items-center justify-between">
                                    <p-skeleton width="4rem" height="2rem" />
                                    <p-skeleton width="6rem" height="1rem" shape="circle" size="3rem" />
                                </div>
                            </div>
                        </div>
                    </div>
                </ng-template>
            </p-dataview>
        </div>
    `,
    standalone: true,
    imports: [DataViewModule, SelectButtonModule, SkeletonModule, FormsModule],
    providers: [ProductService]
})
export class DataviewLoadingDemo implements OnInit {
    private productService = inject(ProductService);
    products = signal<any>([]);
    options: string[] = ['list', 'grid'];

    ngOnInit() {
        this.productService.getProducts().then((data) => this.products.set([...data.slice(0, 12)]));
    }

    counterArray(n: number): any[] {
        return Array(n);
    }
}
```

## Accessibility

Screen Reader The container element that wraps the layout options buttons has a group role whereas each button element uses button role and aria-pressed is updated depending on selection state. Values to describe the buttons are derived from the aria.listView and aria.gridView properties of the locale API respectively. Refer to paginator accessibility documentation for the paginator of the component. Keyboard Support Key Function tab Moves focus to the buttons. space Toggles the checked state of a button.

## API

### DataView

Selector: `p-dataView, p-dataview, p-data-view` · Component, standalone

DataView displays data in grid or list layout with pagination and sorting features.

#### Inputs

| Name | Type | Default | Description |
|---|---|---|---|
| `alwaysShowPaginator` | `boolean` | `true` | Whether to show it even there is only one page. |
| `currentPageReportTemplate` | `string` | `'{currentPage} of {totalPages}'` | Template of the current page report element. Available placeholders are {currentPage},{totalPages},{rows},{first},{last} and {totalRecords} |
| `dt` | `Object \| undefined` | - | Defines scoped design tokens of the component. |
| `emptyMessage` | `string` | `''` | Text to display when there is no data. Defaults to global value in i18n translation configuration. |
| `filterBy` | `string \| undefined` | - | Comma separated list of fields in the object graph to search against. |
| `filterLocale` | `string \| undefined` | - | Locale to use in filtering. The default locale is the host environment's current locale. |
| `first` | `number \| undefined` | `0` | Index of the first row to be displayed. |
| `gridStyleClass` | `string` | `''` | Style class of the grid. |
| `layout` | `"list" \| "grid"` | `'list'` | Defines the layout mode. |
| `lazy` | `boolean \| undefined` | - | Defines if data is loaded and interacted with in lazy manner. |
| `lazyLoadOnInit` | `boolean` | `true` | Whether to call lazy loading on initialization. |
| `loading` | `boolean \| undefined` | - | Displays a loader to indicate data load is in progress. |
| `loadingIcon` | `string \| undefined` | - | The icon to show while indicating data load is in progress. |
| `pageLinks` | `number` | `5` | Number of page links to display in paginator. |
| `paginator` | `boolean \| undefined` | - | When specified as true, enables the pagination. |
| `paginatorDropdownAppendTo` | `any` | - | Target element to attach the paginator dropdown overlay, valid values are "body" or a local ng-template variable of another element (note: use binding with brackets for template variables, e.g. [appendTo]="mydiv" for a div element having #mydiv as variable name). |
| `paginatorDropdownScrollHeight` | `string` | `'200px'` | Paginator dropdown height of the viewport in pixels, a scrollbar is defined if height of list exceeds this value. |
| `paginatorPosition` | `"top" \| "bottom" \| "both"` | `'bottom'` | Position of the paginator. |
| `paginatorStyleClass` | `string \| undefined` | - | Custom style class for paginator |
| `pt` | `DataViewPassThrough` | - | Used to pass attributes to DOM elements inside the component. |
| `ptOptions` | `PassThroughOptions \| undefined` | - | Used to configure passthrough(pt) options of the component. |
| `rows` | `number \| undefined` | - | Number of rows to display per page. |
| `rowsPerPageOptions` | `any[] \| number[] \| undefined` | - | Array of integer/object values to display inside rows per page dropdown of paginator |
| `showCurrentPageReport` | `boolean \| undefined` | - | Whether to display current page report. |
| `showFirstLastIcon` | `boolean` | `true` | When enabled, icons are displayed on paginator to go first and last page. |
| `showJumpToPageDropdown` | `boolean \| undefined` | - | Whether to display a dropdown to navigate to any page. |
| `showPageLinks` | `boolean` | `true` | Whether to show page links. |
| `sortField` | `string \| undefined` | - | Property name of data to use in sorting by default. |
| `sortOrder` | `number \| undefined` | - | Order to sort the data by default. |
| `styleClass` | `string \| undefined` | - | Style class of the component. **Deprecated**: since v20.0.0, use `class` instead. |
| `totalRecords` | `number \| undefined` | - | Number of total records, defaults to length of value when not defined. |
| `trackBy` | `Function` | `(index: number, item: any) => item` | Function to optimize the dom operations by delegating to ngForTrackBy, default algorithm checks for object identity. |
| `unstyled` | `boolean \| undefined` | - | Indicates whether the component should be rendered without styles. |
| `value` | `any[] \| undefined` | - | An array of objects to display. |

#### Outputs

| Name | Payload | Description |
|---|---|---|
| `onChangeLayout` | `DataViewLayoutChangeEvent` | Callback to invoke when changing layout. |
| `onLazyLoad` | `DataViewLazyLoadEvent` | Callback to invoke when paging, sorting or filtering happens in lazy mode. |
| `onPage` | `DataViewPageEvent` | Callback to invoke when pagination occurs. |
| `onSort` | `DataViewSortEvent` | Callback to invoke when sorting occurs. |

#### Templates

Define with `<ng-template #name let-context>` inside the `p-dataView` element.

| Name | Context | Description |
|---|---|---|
| `list` | `DataViewListTemplateContext<any>` | Template for the list layout. |
| `grid` | `DataViewGridTemplateContext<any>` | Template for grid layout. |
| `header` | - | Template for the header section. |
| `emptymessage` | - | Template for the empty message section. |
| `footer` | - | Template for the footer section. |
| `paginatorleft` | `DataViewPaginatorLeftTemplateContext` | Template for the left side of paginator. |
| `paginatorright` | `DataViewPaginatorRightTemplateContext` | Template for the right side of paginator. |
| `paginatordropdownitem` | `DataViewPaginatorDropdownItemTemplateContext` | Template for items in paginator dropdown. |
| `loadingicon` | - | Template for loading icon. |
| `listicon` | - | Template for list icon. |
| `gridicon` | - | Template for grid icon. |

## Pass Through Options

| Name | Type | Description |
|---|---|---|
| `host` | `PassThroughOption<HTMLElement, I>` | Used to pass attributes to the host's DOM element. |
| `root` | `PassThroughOption<HTMLDivElement, I>` | Used to pass attributes to the root's DOM element. |
| `loading` | `PassThroughOption<HTMLDivElement, I>` | Used to pass attributes to the loading container's DOM element. |
| `loadingOverlay` | `PassThroughOption<HTMLDivElement, I>` | Used to pass attributes to the loading overlay's DOM element. |
| `loadingIcon` | `PassThroughOption<SVGElement, I>` | Used to pass attributes to the loading icon's DOM element. |
| `header` | `PassThroughOption<HTMLDivElement, I>` | Used to pass attributes to the header's DOM element. |
| `content` | `PassThroughOption<HTMLDivElement, I>` | Used to pass attributes to the content container's DOM element. |
| `emptyMessage` | `PassThroughOption<HTMLDivElement, I>` | Used to pass attributes to the empty message's DOM element. |
| `footer` | `PassThroughOption<HTMLDivElement, I>` | Used to pass attributes to the footer's DOM element. |
| `pcPaginator` | `PaginatorPassThrough` | Used to pass attributes to the Paginator component. |

## Theming

### CSS Classes

| Class | Description |
|---|---|
| `p-dataview` | Class name of the root element |
| `p-dataview-header` | Class name of the header element |
| `p-dataview-loading` | Class name of the loading element |
| `p-dataview-loading-overlay` | Class name of the loading overlay element |
| `p-dataview-loading-icon` | Class name of the loading icon element |
| `p-dataview-paginator-[position]` | Class name of the paginator element |
| `p-dataview-content` | Class name of the content element |
| `p-dataview-empty-message` | Class name of the empty message element |
| `p-dataview-footer` | Class name of the footer element |

### Design Tokens

| Token | CSS Variable | Description |
|---|---|---|
| `dataview.border.color` | `--p-dataview-border-color` | Border color of root |
| `dataview.border.width` | `--p-dataview-border-width` | Border width of root |
| `dataview.border.radius` | `--p-dataview-border-radius` | Border radius of root |
| `dataview.padding` | `--p-dataview-padding` | Padding of root |
| `dataview.header.background` | `--p-dataview-header-background` | Background of header |
| `dataview.header.color` | `--p-dataview-header-color` | Color of header |
| `dataview.header.border.color` | `--p-dataview-header-border-color` | Border color of header |
| `dataview.header.border.width` | `--p-dataview-header-border-width` | Border width of header |
| `dataview.header.padding` | `--p-dataview-header-padding` | Padding of header |
| `dataview.header.border.radius` | `--p-dataview-header-border-radius` | Border radius of header |
| `dataview.content.background` | `--p-dataview-content-background` | Background of content |
| `dataview.content.color` | `--p-dataview-content-color` | Color of content |
| `dataview.content.border.color` | `--p-dataview-content-border-color` | Border color of content |
| `dataview.content.border.width` | `--p-dataview-content-border-width` | Border width of content |
| `dataview.content.padding` | `--p-dataview-content-padding` | Padding of content |
| `dataview.content.border.radius` | `--p-dataview-content-border-radius` | Border radius of content |
| `dataview.footer.background` | `--p-dataview-footer-background` | Background of footer |
| `dataview.footer.color` | `--p-dataview-footer-color` | Color of footer |
| `dataview.footer.border.color` | `--p-dataview-footer-border-color` | Border color of footer |
| `dataview.footer.border.width` | `--p-dataview-footer-border-width` | Border width of footer |
| `dataview.footer.padding` | `--p-dataview-footer-padding` | Padding of footer |
| `dataview.footer.border.radius` | `--p-dataview-footer-border-radius` | Border radius of footer |
| `dataview.paginator.top.border.color` | `--p-dataview-paginator-top-border-color` | Border color of paginator top |
| `dataview.paginator.top.border.width` | `--p-dataview-paginator-top-border-width` | Border width of paginator top |
| `dataview.paginator.bottom.border.color` | `--p-dataview-paginator-bottom-border-color` | Border color of paginator bottom |
| `dataview.paginator.bottom.border.width` | `--p-dataview-paginator-bottom-border-width` | Border width of paginator bottom |
