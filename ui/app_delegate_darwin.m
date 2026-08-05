//go:build darwin

#import <AppKit/AppKit.h>

extern void appReopened(void);
extern void appShouldTerminate(void);
extern void dockMenuItemClicked(int index);

// Generic action target for Dock menu items - the menu contents (titles,
// separators, count) are fully defined from the Go side. Each item's tag
// is the index of its Go-side callback, dispatched through the single
// dockMenuItemClicked export.
@interface DockMenuTarget : NSObject
- (void)itemClicked:(id)sender;
@end

@implementation DockMenuTarget
- (void)itemClicked:(id)sender {
    dockMenuItemClicked((int)[(NSMenuItem *)sender tag]);
}
@end

// dockMenuBuilding is assembled by dockMenuBegin/AddItem/AddSeparator and
// published to dockMenu (returned from -applicationDockMenu:) by
// dockMenuCommit, so an in-progress rebuild is never seen half-built.
static DockMenuTarget *dockMenuTarget;
static NSMenu *dockMenuBuilding;
static NSMenu *dockMenu;

@interface ReopenDelegate : NSObject<NSApplicationDelegate>
@property (nonatomic, strong) id<NSApplicationDelegate> wrapped;
@end


@implementation ReopenDelegate

- (NSApplicationTerminateReply)applicationShouldTerminate:(NSApplication *)sender {
    // Set the quitting flag before GLFW's delegate fires close requests for
    // each window, so our close intercept knows to close rather than hide.
    appShouldTerminate();
    if ([self.wrapped respondsToSelector:_cmd]) {
        return [self.wrapped applicationShouldTerminate:sender];
    }
    return NSTerminateNow;
}

- (BOOL)applicationShouldHandleReopen:(NSApplication *)app
                    hasVisibleWindows:(BOOL)hasVisible {
    if (!hasVisible) {
        appReopened();
    }
    if ([self.wrapped respondsToSelector:_cmd]) {
        return [self.wrapped applicationShouldHandleReopen:app
                                        hasVisibleWindows:hasVisible];
    }
    return YES;
}

- (NSMenu *)applicationDockMenu:(NSApplication *)sender {
    return dockMenu;
}

- (BOOL)respondsToSelector:(SEL)sel {
    return [super respondsToSelector:sel] ||
           [self.wrapped respondsToSelector:sel];
}

- (id)forwardingTargetForSelector:(SEL)sel {
    return [self.wrapped respondsToSelector:sel] ? self.wrapped : nil;
}

@end

void installReopenDelegate(void) {
    ReopenDelegate *d = [ReopenDelegate new];
    d.wrapped = [NSApp delegate];
    [NSApp setDelegate:d];
}

void dockMenuBegin(void) {
    if (!dockMenuTarget) {
        dockMenuTarget = [DockMenuTarget new];
    }
    dockMenuBuilding = [[NSMenu alloc] initWithTitle:@""];
}

void dockMenuAddItem(const char *title, int index) {
    NSMenuItem *item = [[NSMenuItem alloc] initWithTitle:[NSString stringWithUTF8String:title]
                                                   action:@selector(itemClicked:)
                                            keyEquivalent:@""];
    item.target = dockMenuTarget;
    item.tag = index;
    [dockMenuBuilding addItem:item];
}

void dockMenuAddSeparator(void) {
    [dockMenuBuilding addItem:[NSMenuItem separatorItem]];
}

void dockMenuCommit(void) {
    dockMenu = dockMenuBuilding;
    dockMenuBuilding = nil;
}
