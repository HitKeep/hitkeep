# Virtual Scroller

VirtualScroller is a performance-approach to handle huge data efficiently.

## Import

```typescript
import { ScrollerModule, Scroller } from '@openng/optimus-ui/scroller';
```

## Basic

VirtualScroller requires items as the data to display, itemSize for the dimensions of an item and item template are required on component. In addition, an initial array is required based on the total number of items to display. Size of the viewport is configured using scrollWidth , scrollHeight properties directly or with CSS width and height styles.

## Horizontal

Setting orientation to horizontal enables scrolling horizontally. In this case, the itemSize should refer to the width of an item.

## Grid

Scrolling can be enabled vertically and horizontally when orientation is set as both . In this mode, itemSize should be an array where first value is the height of an item and second is the width.

## Delay

Scroll delay is adjusted by using delay property.

## Programmatic

Scrolling to a specific index can be done with the scrollToIndex function.

## Template

Scroller content is customizable by using ng-template . Valid values are content , item , loader and loadericon

## Scroll Options

The properties of scroller component can be used like an object in it.

## Accessibility

Screen Reader VirtualScroller uses a semantic list element to list the items. No specific role is enforced, still you may use any aria role and attributes as any valid attribute is passed to the container element. Keyboard Support Component does not include any built-in interactive elements.

## Lazyload

Lazy mode is handy to deal with large datasets where instead of loading the entire data, small chunks of data are loaded on demand by invoking onLazyLoad callback everytime scrolling requires a new chunk. To implement lazy loading, enable lazy attribute, initialize your data as a placeholder with a length and finally implement a method callback using onLazyLoad that actually loads a chunk from a datasource. onLazyLoad gets an event object that contains information about the chunk of data to load such as the index and number of items to load. Notice that a new template called loadingItem is also required to display as a placeholder while the new items are being loaded.

## Loader

Busy state is enabled by adding showLoader property which blocks the UI with a modal by default. Alternatively, loader template can be used to customize items e.g. with Skeleton .

## API

### Scroller

Selector: `p-scroller, p-virtualscroller, p-virtual-scroller, p-virtualScroller` · Component, standalone

Scroller is a performance-approach to handle huge data efficiently.

#### Inputs

| Name | Type | Default | Description |
|---|---|---|---|
| `appendOnly` | `boolean` | - | Used to append each loaded item to top without removing any items from the DOM. Using very large data may cause the browser to crash. |
| `autoSize` | `boolean` | - | Defines whether to dynamically change the height or width of scrollable container. |
| `columns` | `any[] \| null \| undefined` | - | Columns to display. |
| `delay` | `number` | - | Delay in scroll before new data is loaded. |
| `disabled` | `boolean` | - | If disabled, the scroller feature is eliminated and the content is displayed directly. |
| `dt` | `Object \| undefined` | - | Defines scoped design tokens of the component. |
| `hostName` | `string` | `''` | - |
| `id` | `string \| undefined` | - | Unique identifier of the element. |
| `inline` | `boolean` | - | Specifies whether the scroller should be displayed inline or not. |
| `items` | `any[] \| null \| undefined` | - | An array of objects to display. |
| `itemSize` | `number \| number[]` | - | The height/width of item according to orientation. |
| `lazy` | `boolean` | - | Defines if data is loaded and interacted with in lazy manner. |
| `loaderDisabled` | `boolean` | - | Used to implement a custom loader instead of using the loader feature in the scroller. |
| `loading` | `boolean \| undefined` | - | Defines whether the data is loaded. |
| `numToleratedItems` | `number` | - | Determines how many additional elements to add to the DOM outside of the view. According to the scrolls made up and down, extra items are added in a certain algorithm in the form of multiples of this number. Default value is half the number of items shown in the view. |
| `options` | `ScrollerOptions \| undefined` | - | Defines whether to use the scroller feature. The properties of scroller component can be used like an object in it. |
| `orientation` | `"vertical" \| "horizontal" \| "both"` | - | The orientation of scrollbar. |
| `pt` | `VirtualScrollerPassThrough` | - | Used to pass attributes to DOM elements inside the component. |
| `ptOptions` | `PassThroughOptions \| undefined` | - | Used to configure passthrough(pt) options of the component. |
| `resizeDelay` | `number` | - | Delay after window's resize finishes. |
| `scrollHeight` | `string \| undefined` | - | Height of the scroll viewport. |
| `scrollWidth` | `string \| undefined` | - | Width of the scroll viewport. |
| `showLoader` | `boolean` | - | Defines whether to show loader. |
| `showSpacer` | `boolean` | - | Used to implement a custom spacer instead of using the spacer feature in the scroller. |
| `step` | `number` | - | Used to specify how many items to load in each load method in lazy mode. |
| `style` | `any` | - | Inline style of the component. |
| `styleClass` | `string \| undefined` | - | Style class of the element. |
| `tabindex` | `number` | - | Index of the element in tabbing order. |
| `trackBy` | `Function` | - | Function to optimize the dom operations by delegating to ngForTrackBy, default algoritm checks for object identity. |
| `unstyled` | `boolean \| undefined` | - | Indicates whether the component should be rendered without styles. |

#### Outputs

| Name | Payload | Description |
|---|---|---|
| `onLazyLoad` | `ScrollerLazyLoadEvent` | Callback to invoke in lazy mode to load new data. |
| `onScroll` | `ScrollerScrollEvent` | Callback to invoke when scroll position changes. |
| `onScrollIndexChange` | `ScrollerScrollIndexChangeEvent` | Callback to invoke when scroll position and item's range in view changes. |

#### Templates

Define with `<ng-template #name let-context>` inside the `p-scroller` element.

| Name | Context | Description |
|---|---|---|
| `content` | `ScrollerContentTemplateContext` | Content template of the component. |
| `item` | `ScrollerItemTemplateContext` | Item template of the component. |
| `loader` | `ScrollerLoaderTemplateContext` | Loader template of the component. |
| `loadericon` | `ScrollerLoaderIconTemplateContext` | Loader icon template of the component. |

## Theming

### CSS Classes

| Class | Description |
|---|---|
| `p-virtualscroller` | Class name of the root element |
| `p-virtualscroller-content` | Class name of the content element |
| `p-virtualscroller-spacer` | Class name of the spacer element |
| `p-virtualscroller-loader` | Class name of the loader element |
| `p-virtualscroller-loading-icon` | Class name of the loading icon element |

### Design Tokens

| Token | CSS Variable | Description |
|---|---|---|
| `virtualscroller.loader.mask.background` | `--p-virtualscroller-loader-mask-background` | Background of loader mask |
| `virtualscroller.loader.mask.color` | `--p-virtualscroller-loader-mask-color` | Color of loader mask |
| `virtualscroller.loader.icon.size` | `--p-virtualscroller-loader-icon-size` | Size of the loader icon |
