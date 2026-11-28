# Tree

Tree is used to display hierarchical data.

## Import

```typescript
import { TreeModule, Tree, UITreeNode } from '@openng/optimus-ui/tree';
import { TreeNode } from '@openng/optimus-ui/api';
```

## Basic

Tree component requires an array of TreeNode objects as its value .

```typescript
import { Component, OnInit, inject, signal } from '@angular/core';
import { TreeModule } from '@openng/optimus-ui/tree';
import { NodeService } from '@/service/nodeservice';
import { TreeNode } from '@openng/optimus-ui/api';

@Component({
    template: `
        <div class="card">
            <p-tree [value]="files()" class="w-full md:w-[30rem]" />
        </div>
    `,
    standalone: true,
    imports: [TreeModule],
    providers: [NodeService]
})
export class TreeBasicDemo implements OnInit {
    private nodeService = inject(NodeService);
    files = signal<TreeNode[]>(undefined);

    ngOnInit() {
        this.nodeService.getFiles().then((data) => {
            this.files.set(data);
        });
    }
}
```

## Controlled

Tree requires a collection of TreeNode instances as a value .

```typescript
import { Component, OnInit, inject, signal } from '@angular/core';
import { ButtonModule } from '@openng/optimus-ui/button';
import { TreeModule } from '@openng/optimus-ui/tree';
import { NodeService } from '@/service/nodeservice';
import { TreeNode } from '@openng/optimus-ui/api';

@Component({
    template: `
        <div class="card">
            <div class="flex flex-wrap gap-2 mb-6">
                <p-button icon="pi pi-plus" label="Expand all" (click)="expandAll()" />
                <p-button icon="pi pi-minus" label="Collapse all" (click)="collapseAll()" />
            </div>
            <p-tree [value]="files()" class="w-full md:w-[30rem]" />
        </div>
    `,
    standalone: true,
    imports: [ButtonModule, TreeModule],
    providers: [NodeService]
})
export class TreeControlledDemo implements OnInit {
    private nodeService = inject(NodeService);
    files = signal<TreeNode[]>(undefined);

    ngOnInit() {
        this.nodeService.getFiles().then((data) => {
            this.files.set(data);
        });
    }

    expandAll() {
        const updatedFiles = this.files().map((node) => this.expandRecursive(node, true));
        this.files.set(updatedFiles);
    }

    collapseAll() {
        const updatedFiles = this.files().map((node) => this.expandRecursive(node, false));
        this.files.set(updatedFiles);
    }
}
```

## Single

Single node selection is configured by setting selectionMode as single along with selection properties to manage the selection value binding.

```typescript
import { Component, OnInit, inject, signal } from '@angular/core';
import { TreeModule } from '@openng/optimus-ui/tree';
import { NodeService } from '@/service/nodeservice';
import { TreeNode } from '@openng/optimus-ui/api';

@Component({
    template: `
        <div class="card">
            <p-tree [value]="files()" class="w-full md:w-[30rem]" selectionMode="single" [(selection)]="selectedFile" />
        </div>
    `,
    standalone: true,
    imports: [TreeModule],
    providers: [NodeService]
})
export class TreeSingleDemo implements OnInit {
    private nodeService = inject(NodeService);
    files = signal<TreeNode[]>(undefined);
    selectedFile!: TreeNode;

    ngOnInit() {
        this.nodeService.getFiles().then((data) => {
            this.files.set(data);
        });
    }
}
```

## Multiple

More than one node is selectable by setting selectionMode to multiple . By default in multiple selection mode, metaKey press (e.g. ⌘ ) is necessary to add to existing selections however this can be configured with disabling the metaKeySelection property. Note that in touch enabled devices, Tree always ignores metaKey. In multiple selection mode, value binding should be a key-value pair where key is the node key and value is a boolean to indicate selection.

```typescript
import { Component, OnInit, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { ToggleSwitchModule } from '@openng/optimus-ui/toggleswitch';
import { Tree, TreeModule } from '@openng/optimus-ui/tree';
import { NodeService } from '@/service/nodeservice';
import { TreeNode } from '@openng/optimus-ui/api';

@Component({
    template: `
        <div class="card">
            <div class="flex items-center mb-6 gap-2">
                <p-toggleswitch inputId="input-metakey" [(ngModel)]="metaKeySelection" />
                <label for="input-metakey">MetaKey</label>
            </div>
            <p-tree [metaKeySelection]="metaKeySelection" [value]="files()" class="w-full md:w-[30rem]" selectionMode="multiple" [(selection)]="selectedFiles" />
        </div>
    `,
    standalone: true,
    imports: [ToggleSwitchModule, TreeModule, FormsModule],
    providers: [NodeService]
})
export class TreeMultipleDemo implements OnInit {
    private nodeService = inject(NodeService);
    metaKeySelection: boolean = false;
    files = signal<TreeNode[]>(undefined);
    selectedFiles!: TreeNode[];

    ngOnInit() {
        this.nodeService.getFiles().then((data) => {
            this.files.set(data);
        });
    }
}
```

## Checkbox

Selection of multiple nodes via checkboxes is enabled by configuring selectionMode as checkbox .

```typescript
import { Component, OnInit, inject, signal } from '@angular/core';
import { TreeModule } from '@openng/optimus-ui/tree';
import { NodeService } from '@/service/nodeservice';
import { TreeNode } from '@openng/optimus-ui/api';

@Component({
    template: `
        <div class="card">
            <p-tree [value]="files()" selectionMode="checkbox" class="w-full md:w-[30rem]" [(selection)]="selectedFiles" />
        </div>
    `,
    standalone: true,
    imports: [TreeModule],
    providers: [NodeService]
})
export class TreeCheckboxDemo implements OnInit {
    private nodeService = inject(NodeService);
    files = signal<TreeNode[]>(undefined);
    selectedFiles!: TreeNode[];

    ngOnInit() {
        this.nodeService.getFiles().then((data) => {
            this.files.set(data);
        });
    }
}
```

## Events

An event is provided for each type of user interaction such as expand, collapse and selection.

```typescript
import { Component, OnInit, inject, signal } from '@angular/core';
import { ToastModule } from '@openng/optimus-ui/toast';
import { TreeModule } from '@openng/optimus-ui/tree';
import { NodeService } from '@/service/nodeservice';
import { TreeNode, MessageService } from '@openng/optimus-ui/api';

@Component({
    template: `
        <div class="card">
            <p-toast />
            <p-tree
                [value]="files()"
                class="w-full md:w-[30rem]"
                selectionMode="single"
                [(selection)]="selectedFile"
                (onNodeExpand)="nodeExpand($event)"
                (onNodeCollapse)="nodeCollapse($event)"
                (onNodeSelect)="nodeSelect($event)"
                (onNodeUnselect)="nodeUnselect($event)"
            />
        </div>
    `,
    standalone: true,
    imports: [ToastModule, TreeModule],
    providers: [NodeService, MessageService]
})
export class TreeEventDemo implements OnInit {
    private nodeService = inject(NodeService);
    private messageService = inject(MessageService);
    files = signal<TreeNode[]>(undefined);
    selectedFile!: TreeNode;

    ngOnInit() {
        this.nodeService.getFiles().then((data) => {
            this.files.set(data);
        });
    }

    nodeExpand(event: any) {
        this.messageService.add({ severity: 'success', summary: 'Node Expanded', detail: event.node.label });
    }

    nodeCollapse(event: any) {
        this.messageService.add({ severity: 'warn', summary: 'Node Collapsed', detail: event.node.label });
    }

    nodeSelect(event: any) {
        this.messageService.add({ severity: 'info', summary: 'Node Selected', detail: event.node.label });
    }

    nodeUnselect(event: any) {
        this.messageService.add({ severity: 'info', summary: 'Node Unselected', detail: event.node.label });
    }
}
```

## Lazy

Lazy loading is useful when dealing with huge datasets, in this example nodes are dynamically loaded on demand using loading property and onNodeExpand method.

```typescript
import { Component, OnInit, signal } from '@angular/core';
import { TreeModule } from '@openng/optimus-ui/tree';
import { TreeNode } from '@openng/optimus-ui/api';

@Component({
    template: `
        <div class="card">
            <p-tree class="w-full md:w-[30rem]" [value]="nodes()" loadingMode="icon" (onNodeExpand)="onNodeExpand($event)" />
        </div>
    `,
    standalone: true,
    imports: [TreeModule]
})
export class TreeLazyDemo implements OnInit {
    nodes = signal<TreeNode[]>(undefined);

    ngOnInit() {
        this.nodes.set(this.initiateNodes());
        setTimeout(() => {
            this.nodes.set(this.nodes().map((node) => ({ ...node, loading: false })));
        }, 2000);
    }

    initiateNodes(): TreeNode[] {
        return [
            {
                key: '0',
                label: 'Node 0',
                leaf: false,
                loading: true
            },
            {
                key: '1',
                label: 'Node 1',
                leaf: false,
                loading: true
            },
            {
                key: '2',
                label: 'Node 2',
                leaf: false,
                loading: true
            }
        ];
    }

    onNodeExpand(event: any) {
        if (!event.node.children) {
            event.node.loading = true;
        
            setTimeout(() => {
                const _nodes = this.nodes();
                let _node = { ...event.node };
                _node.children = [];
        
                for (let i = 0; i < 3; i++) {
                    _node.children.push({
                        key: event.node.key + '-' + i,
                        label: 'Lazy ' + event.node.label + '-' + i
                    });
                }
        
                const key = parseInt(_node.key, 10);
                _nodes[key] = { ..._node, loading: false };
                this.nodes.set([..._nodes]);
            }, 500);
        }
    }
}
```

## Template

Custom node content instead of a node label is defined with the pTemplate property.

```typescript
import { Component, OnInit, signal } from '@angular/core';
import { TreeModule } from '@openng/optimus-ui/tree';
import { TreeNode } from '@openng/optimus-ui/api';

@Component({
    template: `
        <div class="card">
            <p-tree [value]="nodes()" class="w-full md:w-[30rem]">
                <ng-template let-node pTemplate="url">
                    <a [href]="node.data" target="_blank" rel="noopener noreferrer" class="text-surface-700 dark:text-surface-100 hover:text-primary">{{ node.label }}</a>
                </ng-template>
                <ng-template let-node pTemplate="default">
                    <b>{{ node.label }}</b>
                </ng-template>
            </p-tree>
        </div>
    `,
    standalone: true,
    imports: [TreeModule]
})
export class TreeTemplateDemo implements OnInit {
    nodes = signal<TreeNode[]>(undefined);

    ngOnInit() {
        this.nodes.set([
            {
                key: '0',
                label: 'Introduction',
                children: [
                    { key: '0-0', label: 'What is Angular', data: 'https://angular.io', type: 'url' },
                    { key: '0-1', label: 'Getting Started', data: 'https://angular.io/guide/setup-local', type: 'url' },
                    { key: '0-2', label: 'Learn and Explore', data: 'https://angular.io/guide/architecture', type: 'url' },
                    { key: '0-3', label: 'Take a Look', data: 'https://angular.io/start', type: 'url' }
                ]
            },
            {
                key: '1',
                label: 'Components In-Depth',
                children: [
                    { key: '1-0', label: 'Component Registration', data: 'https://angular.io/guide/component-interaction', type: 'url' },
                    { key: '1-1', label: 'User Input', data: 'https://angular.io/guide/user-input', type: 'url' },
                    { key: '1-2', label: 'Hooks', data: 'https://angular.io/guide/lifecycle-hooks', type: 'url' },
                    { key: '1-3', label: 'Attribute Directives', data: 'https://angular.io/guide/attribute-directives', type: 'url' }
                ]
            }
        ]);
    }
}
```

## Filter

Filtering is enabled by adding the filter property, by default label property of a node is used to compare against the value in the text field, in order to customize which field(s) should be used during search define filterBy property. In addition filterMode specifies the filtering strategy. In lenient mode when the query matches a node, children of the node are not searched further as all descendants of the node are included. On the other hand, in strict mode when the query matches a node, filtering continues on all descendants.

```typescript
import { Component, OnInit, inject, signal } from '@angular/core';
import { TreeModule } from '@openng/optimus-ui/tree';
import { NodeService } from '@/service/nodeservice';
import { TreeNode } from '@openng/optimus-ui/api';

@Component({
    template: `
        <div class="card flex flex-wrap gap-4">
            <div class="flex-auto md:flex md:justify-start md:items-center flex-col">
                <p-tree [value]="files()" [filter]="true" filterPlaceholder="Lenient Filter" />
            </div>
            <div class="flex-auto md:flex md:justify-start md:items-center flex-col">
                <p-tree [value]="files2()" [filter]="true" filterMode="strict" filterPlaceholder="Strict Filter" />
            </div>
        </div>
    `,
    standalone: true,
    imports: [TreeModule],
    providers: [NodeService]
})
export class TreeFilterDemo implements OnInit {
    private nodeService = inject(NodeService);
    files = signal<TreeNode[]>(undefined);
    files2 = signal<TreeNode[]>(undefined);

    ngOnInit() {
        this.nodeService.getFiles().then((data) => {
            this.files.set(data);
            this.files2.set(data);
        });
    }
}
```

## Virtual Scroll

VirtualScroller is a performance-approach to handle huge data efficiently. Setting virtualScroll property as true and providing a virtualScrollItemSize in pixels would be enough to enable this functionality.

```typescript
import { Component, OnInit, inject, signal } from '@angular/core';
import { TreeModule } from '@openng/optimus-ui/tree';
import { NodeService } from '@/service/nodeservice';
import { TreeNode } from '@openng/optimus-ui/api';

@Component({
    template: `
        <div class="card">
            <p-tree [value]="nodes()" scrollHeight="250px" [virtualScroll]="true" [virtualScrollItemSize]="35" />
        </div>
    `,
    standalone: true,
    imports: [TreeModule],
    providers: [NodeService]
})
export class TreeVirtualscrollDemo implements OnInit {
    private nodeService = inject(NodeService);
    nodes = signal<TreeNode[]>(undefined);

    ngOnInit() {
        this.nodes.set(this.nodeService.generateNodes(150));
    }
}
```

## Context Menu

Tree has exclusive integration with ContextMenu using the contextMenu property along with the contextMenuSelection to manage the selection.

```typescript
import { Component, OnInit, inject, signal } from '@angular/core';
import { ContextMenu, ContextMenuModule } from '@openng/optimus-ui/contextmenu';
import { ToastModule } from '@openng/optimus-ui/toast';
import { TreeModule } from '@openng/optimus-ui/tree';
import { NodeService } from '@/service/nodeservice';
import { TreeNode, MenuItem, MessageService } from '@openng/optimus-ui/api';
import { ContextMenu } from '@openng/optimus-ui/contextmenu';

@Component({
    template: `
        <div class="card">
            <p-toast [style]="{ marginTop: '80px' }" />
            <p-tree [value]="files()" class="w-full md:w-80" selectionMode="single" [(selection)]="selectedNode" [(contextMenuSelection)]="contextMenuNode" [contextMenu]="cm" contextMenuSelectionMode="separate" />
            <p-contextmenu #cm [model]="items" />
        </div>
    `,
    standalone: true,
    imports: [ContextMenuModule, ToastModule, TreeModule],
    providers: [NodeService, MessageService]
})
export class TreeContextmenuDemo implements OnInit {
    private nodeService = inject(NodeService);
    private messageService = inject(MessageService);
    files = signal<TreeNode[]>([]);
    selectedNode: any = model<TreeNode | null>(null);
    contextMenuNode: any = model<TreeNode | null>(null);
    items!: MenuItem[];

    ngOnInit() {
        this.nodeService.getFiles().then((files) => this.files.set(files));
        this.items = [
            { label: 'View', icon: 'pi pi-search', command: () => this.viewFile(this.contextMenuNode()) },
            { label: 'Toggle', icon: 'pi pi-sort', command: () => this.toggleFile(this.contextMenuNode()) }
        ];
    }

    viewFile(node: TreeNode | null) {
        if (node) {
            this.messageService.add({ severity: 'info', summary: 'File Selected', detail: node.label });
        }
    }

    toggleFile(node: TreeNode | null) {
        if (node) {
            this.files.set(this.updateNodeInTree(this.files(), node.key, { ...node, expanded: !node.expanded }));
        }
    }

    updateNodeInTree(nodes: TreeNode[], key: string | undefined, updatedNode: TreeNode): TreeNode[] {
        return nodes.map((n) => {
            if (n.key === key) {
                return updatedNode;
            }
            if (n.children) {
                return { ...n, children: this.updateNodeInTree(n.children, key, updatedNode) };
            }
            return n;
        });
    }
}
```

## Accessibility

Screen Reader Value to describe the component can either be provided with aria-labelledby or aria-label props. The root list element has a tree role whereas each list item has a treeitem role along with aria-label , aria-selected and aria-expanded attributes. In checkbox selection, aria-checked is used instead of aria-selected . The container element of a treenode has the group role. Checkbox and toggle icons are hidden from screen readers as their parent element with treeitem role and attributes are used instead for readers and keyboard support. The aria-setsize , aria-posinset and aria-level attributes are calculated implicitly and added to each treeitem. Keyboard Support Key Function tab Moves focus to the first selected node when focus enters the component, if there is none then first element receives the focus. If focus is already inside the component, moves focus to the next focusable element in the page tab sequence. shift + tab Moves focus to the last selected node when focus enters the component, if there is none then first element receives the focus. If focus is already inside the component, moves focus to the previous focusable element in the page tab sequence. enter Selects the focused treenode. space Selects the focused treenode. down arrow Moves focus to the next treenode. up arrow Moves focus to the previous treenode. right arrow If node is closed, opens the node otherwise moves focus to the first child node. left arrow If node is open, closes the node otherwise moves focus to the parent node.

## Virtualscrolllazy

VirtualScroller is a performance-approach to handle huge data efficiently. Setting virtualScroll property as true and providing a virtualScrollItemSize in pixels would be enough to enable this functionality.

```typescript
import { Component, OnInit, inject, signal } from '@angular/core';
import { TreeModule } from '@openng/optimus-ui/tree';
import { NodeService } from '@/service/nodeservice';
import { TreeNode } from '@openng/optimus-ui/api';

@Component({
    template: `
        <div class="card">
            <p-tree [value]="nodes()" scrollHeight="250px" [virtualScroll]="true" [lazy]="true" [virtualScrollItemSize]="35" (onNodeExpand)="nodeExpand($event)" [loading]="loading()" />
        </div>
    `,
    standalone: true,
    imports: [TreeModule],
    providers: [NodeService]
})
export class TreeVirtualscrolllazyDemo implements OnInit {
    private nodeService = inject(NodeService);
    loading = signal<boolean>(false);
    nodes = signal<TreeNode[]>(undefined);

    ngOnInit() {
        this.loading.set(true);
        setTimeout(() => {
            this.nodes.set(this.nodeService.generateNodes(150));
            this.loading.set(false);
        }, 1000);
    }

    nodeExpand(event: any) {
        if (event.node) {
            this.loading.set(true);
            setTimeout(() => {
                event.node.children = this.nodeService.createNodes(5, event.node.key);
                this.loading.set(false);
                this.nodes.set([...this.nodes()]);
            }, 200);
        }
    }
}
```

## API

### Tree

Selector: `p-tree` · Component, standalone

Tree is used to display hierarchical data.

#### Inputs

| Name | Type | Default | Description |
|---|---|---|---|
| `_templateMap` | `any` | - | Custom templates of the component. |
| `ariaLabel` | `string \| undefined` | - | Used to define a string that labels the tree. |
| `ariaLabelledBy` | `string \| undefined` | - | Establishes relationships between the component and label(s) where its value should be one or more element IDs. |
| `contextMenu` | `any` | - | Context menu instance. |
| `contextMenuSelection` | `TreeNode<any> \| null` | - | Selected node with a context menu. Two-way: `[(contextMenuSelection)]`. |
| `contextMenuSelectionMode` | `"separate" \| "joint"` | `'joint'` | Defines the behavior of context menu selection, in "separate" mode context menu updates contextMenuSelection property whereas in joint mode selection property is used instead so that when row selection is enabled, both row selection and context menu selection use the same property. |
| `draggableNodes` | `boolean \| undefined` | - | Whether the nodes are draggable. |
| `draggableScope` | `any` | - | Scope of the draggable nodes to match a droppableScope. |
| `droppableNodes` | `boolean \| undefined` | - | Whether the nodes are droppable. |
| `droppableScope` | `any` | - | Scope of the droppable nodes to match a draggableScope. |
| `dt` | `Object \| undefined` | - | Defines scoped design tokens of the component. |
| `emptyMessage` | `string` | `''` | Text to display when there is no data. |
| `filter` | `boolean \| undefined` | - | When specified, displays an input field to filter the items. |
| `filterBy` | `string` | `'label'` | When filtering is enabled, filterBy decides which field or fields (comma separated) to search against. |
| `filteredNodes` | `TreeNode<any>[] \| null \| undefined` | - | Values after the tree nodes are filtered. |
| `filterInputAutoFocus` | `boolean` | `false` | Determines whether the filter input should be automatically focused when the component is rendered. |
| `filterLocale` | `string \| undefined` | - | Locale to use in filtering. The default locale is the host environment's current locale. |
| `filterMode` | `string` | `'lenient'` | Mode for filtering valid values are "lenient" and "strict". Default is lenient. |
| `filterOptions` | `any` | - | Mode for filtering valid values are "lenient" and "strict". Default is lenient. |
| `filterPlaceholder` | `string \| undefined` | - | Placeholder text to show when filter input is empty. |
| `highlightOnSelect` | `boolean` | `false` | Highlights the node on select. |
| `indentation` | `number` | `1.5` | Indentation factor for spacing of the nested node when virtual scrolling is enabled. |
| `lazy` | `boolean` | `false` | Defines if data is loaded and interacted with in lazy manner. |
| `loading` | `boolean \| undefined` | - | Displays a loader to indicate data load is in progress. |
| `loadingIcon` | `string \| undefined` | - | The icon to show while indicating data load is in progress. |
| `loadingMode` | `"icon" \| "mask"` | `'mask'` | Loading mode display. |
| `metaKeySelection` | `boolean` | `false` | Defines how multiple items can be selected, when true metaKey needs to be pressed to select or unselect an item and when set to false selection of each item can be toggled individually. On touch enabled devices, metaKeySelection is turned off automatically. |
| `propagateSelectionDown` | `boolean` | `true` | Whether checkbox selections propagate to descendant nodes. |
| `propagateSelectionUp` | `boolean` | `true` | Whether checkbox selections propagate to ancestor nodes. |
| `pt` | `TreePassThrough` | - | Used to pass attributes to DOM elements inside the component. |
| `ptOptions` | `PassThroughOptions \| undefined` | - | Used to configure passthrough(pt) options of the component. |
| `scrollHeight` | `string \| undefined` | - | Height of the scrollable viewport. |
| `selection` | `TreeNode<any> \| TreeNode<any>[] \| null \| undefined` | - | A single treenode instance or an array to refer to the selections. Two-way: `[(selection)]`. |
| `selectionMode` | `"single" \| "multiple" \| "checkbox"` | - | Defines the selection mode. |
| `styleClass` | `string \| undefined` | - | Style class of the component. **Deprecated**: since v20.0.0, use `class` instead. |
| `togglerAriaLabel` | `string \| undefined` | - | Defines a string that labels the toggler icon for accessibility. |
| `trackBy` | `Function` | `(index: number, item: any) => item` | Function to optimize the node list rendering, default algorithm checks for object identity. |
| `unstyled` | `boolean \| undefined` | - | Indicates whether the component should be rendered without styles. |
| `validateDrop` | `boolean \| undefined` | - | When enabled, drop can be accepted or rejected based on condition defined at onNodeDrop. |
| `value` | `any` | - | An array of treenodes. |
| `virtualScroll` | `boolean \| undefined` | - | Whether the data should be loaded on demand during scroll. |
| `virtualScrollItemSize` | `number \| undefined` | - | Height of an item in the list for VirtualScrolling. |
| `virtualScrollOptions` | `ScrollerOptions \| undefined` | - | Whether to use the scroller feature. The properties of scroller component can be used like an object in it. |

#### Outputs

| Name | Payload | Description |
|---|---|---|
| `contextMenuSelectionChange` | `TreeNode<any> \| null` | Selected node with a context menu. |
| `onFilter` | `TreeFilterEvent` | Callback to invoke when data is filtered. |
| `onLazyLoad` | `TreeLazyLoadEvent` | Callback to invoke in lazy mode to load new data. |
| `onNodeCollapse` | `TreeNodeCollapseEvent` | Callback to invoke when a node is collapsed. |
| `onNodeContextMenuSelect` | `TreeNodeContextMenuSelectEvent` | Callback to invoke when a node is selected with right click. |
| `onNodeDoubleClick` | `TreeNodeDoubleClickEvent` | Callback to invoke when a node is double clicked. |
| `onNodeDrop` | `TreeNodeDropEvent` | Callback to invoke when a node is dropped. |
| `onNodeExpand` | `TreeNodeExpandEvent` | Callback to invoke when a node is expanded. |
| `onNodeSelect` | `TreeNodeSelectEvent` | Callback to invoke when a node is selected. |
| `onNodeUnselect` | `TreeNodeUnSelectEvent` | Callback to invoke when a node is unselected. |
| `onScroll` | `TreeScrollEvent` | Callback to invoke in virtual scroll mode when scroll position changes. |
| `onScrollIndexChange` | `TreeScrollIndexChangeEvent` | Callback to invoke in virtual scroll mode when scroll position and item's range in view changes. |
| `selectionChange` | `TreeNode<any> \| TreeNode<any>[] \| null \| undefined` | A single treenode instance or an array to refer to the selections. |

#### Templates

Define with `<ng-template #name let-context>` inside the `p-tree` element.

| Name | Context | Description |
|---|---|---|
| `filter` | `TreeFilterTemplateContext` | Custom filter template. |
| `node` | `any` | Custom node template. |
| `header` | - | Custom header template. |
| `footer` | - | Custom footer template. |
| `loader` | `TreeLoaderTemplateContext` | Custom loader template. |
| `empty` | - | Custom empty message template. |
| `togglericon` | `TreeTogglerIconTemplateContext` | Custom toggler icon template. |
| `checkboxicon` | `TreeCheckboxIconTemplateContext` | Custom checkbox icon template. |
| `loadingicon` | - | Custom loading icon template. |
| `filtericon` | - | Custom filter icon template. |

#### Methods

| Name | Parameters | Returns | Description |
|---|---|---|---|
| `resetFilter` | - | `void` | Resets filter. |
| `scrollToVirtualIndex` | `index: number` | `void` | Scrolls to virtual index. |
| `scrollTo` | `options: any` | `void` | Scrolls to virtual index. |

### UITreeNode

Selector: `p-treeNode` · Component, standalone

#### Inputs

| Name | Type | Default | Description |
|---|---|---|---|
| `dt` | `Object \| undefined` | - | Defines scoped design tokens of the component. |
| `firstChild` | `boolean \| undefined` | - | - |
| `indentation` | `number \| undefined` | - | - |
| `index` | `number \| undefined` | - | - |
| `itemSize` | `number \| undefined` | - | - |
| `lastChild` | `boolean \| undefined` | - | - |
| `level` | `number \| undefined` | - | - |
| `loadingMode` | `string` | - | - |
| `node` | `TreeNode<any> \| undefined` | - | - |
| `parentNode` | `TreeNode<any> \| undefined` | - | - |
| `pt` | `TreePassThrough` | - | Used to pass attributes to DOM elements inside the component. |
| `ptOptions` | `PassThroughOptions \| undefined` | - | Used to configure passthrough(pt) options of the component. |
| `root` | `boolean \| undefined` | - | - |
| `rowNode` | `any` | - | - |
| `unstyled` | `boolean \| undefined` | - | Indicates whether the component should be rendered without styles. |

### TreeNode

Represents a node in a tree data structure.

| Name | Type | Description |
|---|---|---|
| `checked?` | `boolean \| undefined` | - |
| `label?` | `string \| undefined` | Label of the node. |
| `data?` | `T \| undefined` | Data represented by the node. |
| `icon?` | `string \| undefined` | Icon of the node to display next to content. |
| `expandedIcon?` | `string \| undefined` | Icon to use in expanded state. |
| `collapsedIcon?` | `string \| undefined` | Icon to use in collapsed state. |
| `children?` | `TreeNode<T>[] \| undefined` | An array of treenodes as children. |
| `leaf?` | `boolean \| undefined` | Specifies if the node has children. Used in lazy loading. |
| `expanded?` | `boolean \| undefined` | Expanded state of the node. |
| `type?` | `string \| undefined` | Type of the node to match a template. |
| `parent?` | `TreeNode<T> \| undefined` | Parent of the node. |
| `partialSelected?` | `boolean \| undefined` | Defines if value is partially selected. |
| `style?` | `any` | Inline style of the node. |
| `styleClass?` | `string \| undefined` | Style class of the node. |
| `draggable?` | `boolean \| undefined` | Defines if the node is draggable. |
| `droppable?` | `boolean \| undefined` | Defines if the node is droppable. |
| `selectable?` | `boolean \| undefined` | Whether the node is selectable when selection mode is enabled. |
| `key?` | `string \| undefined` | Mandatory unique key of the node. |
| `loading?` | `boolean \| undefined` | Whether the node is loading. Used in lazy loading. |

## Pass Through Options

| Name | Type | Description |
|---|---|---|
| `host` | `PassThroughOption<HTMLElement, I>` | Used to pass attributes to the host's DOM element. |
| `root` | `PassThroughOption<HTMLDivElement, I>` | Used to pass attributes to the root's DOM element. |
| `mask` | `PassThroughOption<HTMLDivElement, I>` | Used to pass attributes to the loading mask's DOM element. |
| `loadingIcon` | `PassThroughOption<HTMLElement, I>` | Used to pass attributes to the loading icon's DOM element. |
| `pcFilterContainer` | `IconFieldPassThrough` | Used to pass attributes to the filter container's DOM element. |
| `pcFilterIconContainer` | `InputIconPassThrough` | Used to pass attributes to the filter icon container's DOM element. |
| `pcFilterInput` | `InputTextPassThrough` | Used to pass attributes to the filter input's DOM element. |
| `filterIcon` | `PassThroughOption<HTMLElement, I>` | Used to pass attributes to the filter icon's DOM element. |
| `pcScroller` | `VirtualScrollerPassThrough` | Used to pass attributes to the Scroller component. |
| `wrapper` | `PassThroughOption<HTMLDivElement, I>` | Used to pass attributes to the wrapper's DOM element. |
| `rootChildren` | `PassThroughOption<HTMLUListElement, I>` | Used to pass attributes to the root children's DOM element. |
| `node` | `PassThroughOption<HTMLLIElement, I>` | Used to pass attributes to the node's DOM element. |
| `dropPoint` | `PassThroughOption<HTMLDivElement, I>` | Used to pass attributes to the drop point's DOM element. |
| `nodeContent` | `PassThroughOption<HTMLDivElement, I>` | Used to pass attributes to the node content's DOM element. |
| `nodeToggleButton` | `PassThroughOption<HTMLButtonElement, I>` | Used to pass attributes to the node toggle button's DOM element. |
| `nodeTogglerIcon` | `PassThroughOption<HTMLElement, I>` | Used to pass attributes to the node toggle icon's DOM element. |
| `pcNodeCheckbox` | `CheckboxPassThrough` | Used to pass attributes to the node checkbox's DOM element. |
| `nodeIcon` | `PassThroughOption<HTMLSpanElement, I>` | Used to pass attributes to the node icon's DOM element. |
| `nodeLabel` | `PassThroughOption<HTMLSpanElement, I>` | Used to pass attributes to the node label's DOM element. |
| `nodeChildren` | `PassThroughOption<HTMLUListElement, I>` | Used to pass attributes to the node children's DOM element. |
| `emptyMessage` | `PassThroughOption<HTMLDivElement, I>` | Used to pass attributes to the empty message's DOM element. |

## Theming

### CSS Classes

| Class | Description |
|---|---|
| `p-tree` | Class name of the root element |
| `p-tree-mask` | Class name of the mask element |
| `p-tree-loading-icon` | Class name of the loading icon element |
| `p-tree-filter-input` | Class name of the filter input element |
| `p-tree-root` | Class name of the wrapper element |
| `p-tree-root-children` | Class name of the root children element |
| `p-tree-node` | Class name of the node element |
| `p-tree-node-content` | Class name of the node content element |
| `p-tree-node-toggle-button` | Class name of the node toggle button element |
| `p-tree-node-toggle-icon` | Class name of the node toggle icon element |
| `p-tree-node-checkbox` | Class name of the node checkbox element |
| `p-tree-node-icon` | Class name of the node icon element |
| `p-tree-node-label` | Class name of the node label element |
| `p-tree-node-children` | Class name of the node children element |
| `p-tree-empty-message` | Class name of the empty message element |
| `p-tree-node-droppoint` | Class name of the drop point element |

### Design Tokens

| Token | CSS Variable | Description |
|---|---|---|
| `tree.background` | `--p-tree-background` | Background of root |
| `tree.color` | `--p-tree-color` | Color of root |
| `tree.padding` | `--p-tree-padding` | Padding of root |
| `tree.gap` | `--p-tree-gap` | Gap of root |
| `tree.indent` | `--p-tree-indent` | Indent of root |
| `tree.transition.duration` | `--p-tree-transition-duration` | Transition duration of root |
| `tree.node.padding` | `--p-tree-node-padding` | Padding of node |
| `tree.node.border.radius` | `--p-tree-node-border-radius` | Border radius of node |
| `tree.node.hover.background` | `--p-tree-node-hover-background` | Hover background of node |
| `tree.node.selected.background` | `--p-tree-node-selected-background` | Selected background of node |
| `tree.node.color` | `--p-tree-node-color` | Color of node |
| `tree.node.hover.color` | `--p-tree-node-hover-color` | Hover color of node |
| `tree.node.selected.color` | `--p-tree-node-selected-color` | Selected color of node |
| `tree.node.focus.ring.width` | `--p-tree-node-focus-ring-width` | Focus ring width of node |
| `tree.node.focus.ring.style` | `--p-tree-node-focus-ring-style` | Focus ring style of node |
| `tree.node.focus.ring.color` | `--p-tree-node-focus-ring-color` | Focus ring color of node |
| `tree.node.focus.ring.offset` | `--p-tree-node-focus-ring-offset` | Focus ring offset of node |
| `tree.node.focus.ring.shadow` | `--p-tree-node-focus-ring-shadow` | Focus ring shadow of node |
| `tree.node.gap` | `--p-tree-node-gap` | Gap of node |
| `tree.node.icon.color` | `--p-tree-node-icon-color` | Color of node icon |
| `tree.node.icon.hover.color` | `--p-tree-node-icon-hover-color` | Hover color of node icon |
| `tree.node.icon.selected.color` | `--p-tree-node-icon-selected-color` | Selected color of node icon |
| `tree.node.toggle.button.border.radius` | `--p-tree-node-toggle-button-border-radius` | Border radius of node toggle button |
| `tree.node.toggle.button.size` | `--p-tree-node-toggle-button-size` | Size of node toggle button |
| `tree.node.toggle.button.hover.background` | `--p-tree-node-toggle-button-hover-background` | Hover background of node toggle button |
| `tree.node.toggle.button.selected.hover.background` | `--p-tree-node-toggle-button-selected-hover-background` | Selected hover background of node toggle button |
| `tree.node.toggle.button.color` | `--p-tree-node-toggle-button-color` | Color of node toggle button |
| `tree.node.toggle.button.hover.color` | `--p-tree-node-toggle-button-hover-color` | Hover color of node toggle button |
| `tree.node.toggle.button.selected.hover.color` | `--p-tree-node-toggle-button-selected-hover-color` | Selected hover color of node toggle button |
| `tree.node.toggle.button.focus.ring.width` | `--p-tree-node-toggle-button-focus-ring-width` | Focus ring width of node toggle button |
| `tree.node.toggle.button.focus.ring.style` | `--p-tree-node-toggle-button-focus-ring-style` | Focus ring style of node toggle button |
| `tree.node.toggle.button.focus.ring.color` | `--p-tree-node-toggle-button-focus-ring-color` | Focus ring color of node toggle button |
| `tree.node.toggle.button.focus.ring.offset` | `--p-tree-node-toggle-button-focus-ring-offset` | Focus ring offset of node toggle button |
| `tree.node.toggle.button.focus.ring.shadow` | `--p-tree-node-toggle-button-focus-ring-shadow` | Focus ring shadow of node toggle button |
| `tree.loading.icon.size` | `--p-tree-loading-icon-size` | Size of loading icon |
| `tree.filter.margin` | `--p-tree-filter-margin` | Margin of filter |
