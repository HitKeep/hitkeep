import { ComponentFixture, TestBed } from '@angular/core/testing';
import { Component } from '@angular/core';
import { By } from '@angular/platform-browser';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { Menu } from '@openng/optimus-ui/menu';
import { firstValueFrom } from 'rxjs';
import { vi } from 'vitest';

import { TableRowActions, TableRowActionItem } from './table-row-actions';

describe('TableRowActions', () => {
    let fixture: ComponentFixture<TableRowActions>;
    let ownedMenus: Set<Menu>;
    const editCommand = vi.fn();
    const deleteCommand = vi.fn();

    beforeEach(async () => {
        editCommand.mockReset();
        deleteCommand.mockReset();
        ownedMenus = new Set();

        await TestBed.configureTestingModule({
            imports: [
                TableRowActions,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            common: {
                                actions: {
                                    more: 'More actions'
                                }
                            }
                        }
                    },
                    translocoConfig: {
                        availableLangs: ['en'],
                        defaultLang: 'en'
                    },
                    preloadLangs: true
                })
            ]
        }).compileComponents();

        fixture = TestBed.createComponent(TableRowActions);
        fixture.componentRef.setInput('items', rowItems());
        await fixture.whenStable();
    });

    afterEach(async () => {
        for (const menu of ownedMenus) {
            if (menu.overlayVisible) {
                const hidden = firstValueFrom(menu.onHide);
                menu.hide();
                await fixture.whenStable();
                await hidden;
            }
        }
    });

    it('renders an accessible ellipsis trigger', () => {
        const button = triggerButton();

        expect(button.getAttribute('aria-label')).toBe('More actions');
        expect(button.querySelector('.pi-ellipsis-h')).not.toBeNull();
    });

    it('opens an OptimusUI menu and executes item commands without onHide', async () => {
        const menu = await openMenu();
        expect(menuElement(menu)?.textContent).toContain('Edit');
        expect(menuElement(menu)?.textContent).toContain('Delete');

        await clickEdit(menu);

        expect(editCommand).toHaveBeenCalledTimes(1);
    });

    it('executes the current menu command when an inert overlay precedes it in the document', async () => {
        const menu = await openMenu();
        const staleOverlay = menuElement(menu)!.cloneNode(true) as HTMLElement;
        staleOverlay.id = `${menu.id}_stale`;
        document.body.prepend(staleOverlay);

        try {
            expect(document.querySelector('.table-row-actions-menu')).toBe(staleOverlay);
            await clickEdit(menu);
            expect(editCommand).toHaveBeenCalledTimes(1);
        } finally {
            staleOverlay.remove();
        }
    });

    it('reopens after a command runs from the menu', async () => {
        const menu = await openMenu();
        await clickEdit(menu);
        await openMenu();

        expect(editCommand).toHaveBeenCalledTimes(1);
        expect(menuElement(menu)?.textContent).toContain('Edit');
    });

    it('supports disabled state', async () => {
        fixture.componentRef.setInput('disabled', true);
        await fixture.whenStable();

        expect(triggerButton().disabled).toBe(true);
    });

    it('applies danger styling to destructive menu items', async () => {
        const menu = await openMenu();

        const dangerItem = menuElement(menu)?.querySelector('.table-row-actions-menu__item--danger');

        expect(dangerItem?.textContent).toContain('Delete');
    });

    it('opens a different row menu on the first click when another row menu is already open', async () => {
        const hostFixture = TestBed.createComponent(TableRowActionsHost);
        await hostFixture.whenStable();
        const first = await openMenu(hostFixture);

        expect(menuElement(first)?.textContent).toContain('First');

        const firstHidden = firstValueFrom(first.onHide);
        const second = await openMenu(hostFixture, 1);
        await firstHidden;
        await hostFixture.whenStable();

        const menus = [menuElement(first), menuElement(second)].filter((menu): menu is HTMLElement => menu !== null);
        expect(menus.filter((menu) => menu.textContent?.includes('Second')).length).toBe(1);
        expect(menus.some((menu) => menu.textContent?.includes('First'))).toBe(false);
    });

    it('closes an open menu when the same trigger is clicked again', async () => {
        const menu = await openMenu();

        expect(menuElement(menu)?.textContent).toContain('Edit');

        const hidden = firstValueFrom(menu.onHide);
        triggerButton().click();
        await fixture.whenStable();
        await hidden;
        await fixture.whenStable();

        expect(menuElement(menu)?.textContent ?? '').not.toContain('Edit');
    });

    async function openMenu(currentFixture: ComponentFixture<unknown> = fixture, index = 0): Promise<Menu> {
        const menu = currentFixture.debugElement.queryAll(By.directive(Menu))[index].componentInstance as Menu;
        ownedMenus.add(menu);
        const shown = firstValueFrom(menu.onShow);
        (currentFixture.nativeElement.querySelectorAll('button')[index] as HTMLButtonElement).click();
        await currentFixture.whenStable();
        await shown;
        expect(menuElement(menu)?.isConnected).toBe(true);
        return menu;
    }

    async function clickEdit(menu: Menu): Promise<void> {
        // Command closure must not depend on OptimusUI delivering onHide.
        const onHide = vi.spyOn(menu.onHide, 'emit').mockImplementation(() => undefined);
        const itemClick = vi.spyOn(menu, 'itemClick');
        try {
            // Temporary CI diagnostics: trace command delivery without changing the real click path.
            const selected = menuElement(menu);
            const owned = menu.containerViewChild()?.nativeElement;
            const anchor = selected?.querySelector('a') as HTMLElement;
            const editItem = menu.model?.[0];
            console.info(
                'row-command-before',
                JSON.stringify({
                    sameElement: selected === owned,
                    copies: document.querySelectorAll(`[id="${menu.id}"]`).length,
                    selectedConnected: selected?.isConnected,
                    ownedConnected: owned?.isConnected,
                    anchorText: anchor?.textContent?.trim(),
                    anchorId: anchor?.id,
                    itemRole: anchor?.closest('[role]')?.getAttribute('role'),
                    itemDisabled: anchor?.closest('[aria-disabled]')?.getAttribute('aria-disabled'),
                    inert: anchor?.closest('[inert]') !== null,
                    visible: menu.visible,
                    overlayVisible: menu.overlayVisible,
                    model: menu.model?.map((item) => ({ label: item.label, disabled: item.disabled, command: typeof item.command }))
                })
            );
            anchor.click();
            const clickedItem = itemClick.mock.calls[0]?.[0]?.item;
            console.info(
                'row-command-sync',
                JSON.stringify({
                    itemClicks: itemClick.mock.calls.length,
                    commands: editCommand.mock.calls.length,
                    sameItem: clickedItem === editItem,
                    sameCommand: clickedItem?.command === editItem?.command,
                    visible: menu.visible
                })
            );
            await fixture.whenStable();
            console.info('row-command-stable', JSON.stringify({ itemClicks: itemClick.mock.calls.length, commands: editCommand.mock.calls.length, visible: menu.visible }));
            expect(editCommand).toHaveBeenCalledTimes(1);
            expect(deleteCommand).not.toHaveBeenCalled();
            expect(menu.visible).toBe(false);
            await expect.poll(() => menuElement(menu)).toBeNull();
        } finally {
            itemClick.mockRestore();
            onHide.mockRestore();
        }
    }

    function menuElement(menu: Menu): HTMLElement | null {
        return document.getElementById(menu.id!);
    }

    function triggerButton(): HTMLButtonElement {
        return fixture.nativeElement.querySelector('button');
    }

    function rowItems(): TableRowActionItem[] {
        return [{ label: 'Edit', icon: 'pi pi-pencil', command: editCommand }, { separator: true }, { label: 'Delete', icon: 'pi pi-trash', danger: true, command: deleteCommand }];
    }
});

@Component({
    imports: [TableRowActions],
    template: `
        <app-table-row-actions [items]="firstItems()" />
        <app-table-row-actions [items]="secondItems()" />
    `
})
class TableRowActionsHost {
    firstItems(): TableRowActionItem[] {
        return [{ label: 'First', icon: 'pi pi-pencil', command: () => undefined }];
    }

    secondItems(): TableRowActionItem[] {
        return [{ label: 'Second', icon: 'pi pi-pencil', command: () => undefined }];
    }
}
