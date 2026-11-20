#import <AppKit/AppKit.h>
#import <WebKit/WebKit.h>
#include <pthread.h>

@interface BacklogMenu : NSObject <WKNavigationDelegate>
@property (strong) NSURL *dashboardURL;
@property (strong) NSStatusItem *statusItem;
@property (strong) NSWindow *window;
@property (strong) WKWebView *webView;
@property (strong) NSTextField *message;
@end

@implementation BacklogMenu

- (BOOL)install {
    self.statusItem = [NSStatusBar.systemStatusBar statusItemWithLength:NSVariableStatusItemLength];
    NSStatusBarButton *button = self.statusItem.button;
    if (button == nil) {
        return NO;
    }
    NSImage *ticket = [NSImage imageWithSystemSymbolName:@"ticket" accessibilityDescription:@"Backlog"];
    ticket.template = YES;
    button.image = ticket;
    if (ticket == nil) {
        button.title = @"Backlog";
    }
    button.toolTip = @"Backlog";
    [button setAccessibilityLabel:@"Backlog"];

    NSMenu *menu = [[NSMenu alloc] initWithTitle:@"Backlog"];
    NSMenuItem *open = [menu addItemWithTitle:@"Open Backlog" action:@selector(openBacklog:) keyEquivalent:@""];
    open.target = self;
    NSMenuItem *quick = [menu addItemWithTitle:@"Quick Backlog" action:@selector(quickBacklog:) keyEquivalent:@""];
    quick.target = self;
    self.statusItem.menu = menu;
    return YES;
}

- (void)openBacklog:(id)sender {
    if (![NSWorkspace.sharedWorkspace openURL:self.dashboardURL]) {
        NSAlert *alert = [[NSAlert alloc] init];
        alert.messageText = @"Could not open Backlog in the default browser.";
        alert.informativeText = self.dashboardURL.absoluteString;
        [alert runModal];
    }
}

- (void)createWindow {
    NSRect frame = NSMakeRect(0, 0, 460, 620);
    self.window = [[NSWindow alloc] initWithContentRect:frame
                                              styleMask:NSWindowStyleMaskTitled | NSWindowStyleMaskClosable | NSWindowStyleMaskResizable
                                                backing:NSBackingStoreBuffered
                                                  defer:NO];
    self.window.title = @"Quick Backlog";
    self.window.releasedWhenClosed = NO;
    self.window.contentMinSize = NSMakeSize(360, 300);
    NSScreen *screen = self.statusItem.button.window.screen ?: NSScreen.mainScreen;
    if (screen != nil) {
        NSRect bounds = screen.visibleFrame;
        [self.window setFrameTopLeftPoint:NSMakePoint(NSMaxX(bounds) - NSWidth(frame) - 12, NSMaxY(bounds) - 8)];
    }

    WKWebViewConfiguration *configuration = [[WKWebViewConfiguration alloc] init];
    configuration.websiteDataStore = WKWebsiteDataStore.nonPersistentDataStore;
    self.webView = [[WKWebView alloc] initWithFrame:frame configuration:configuration];
    self.webView.navigationDelegate = self;
    self.webView.autoresizingMask = NSViewWidthSizable | NSViewHeightSizable;
    NSView *content = [[NSView alloc] initWithFrame:frame];
    self.window.contentView = content;
    [content addSubview:self.webView];

    self.message = [NSTextField wrappingLabelWithString:@"Loading Backlog…"];
    self.message.alignment = NSTextAlignmentCenter;
    self.message.translatesAutoresizingMaskIntoConstraints = NO;
    [content addSubview:self.message];
    [NSLayoutConstraint activateConstraints:@[
        [self.message.centerXAnchor constraintEqualToAnchor:content.centerXAnchor],
        [self.message.centerYAnchor constraintEqualToAnchor:content.centerYAnchor],
        [self.message.widthAnchor constraintLessThanOrEqualToAnchor:content.widthAnchor multiplier:0.8],
    ]];
}

- (void)quickBacklog:(id)sender {
    if (self.window == nil) {
        [self createWindow];
    }
    if (!self.message.hidden && !self.webView.loading) {
        self.message.stringValue = @"Loading Backlog…";
        self.webView.hidden = YES;
        NSURLRequest *request = [NSURLRequest requestWithURL:self.dashboardURL
                                                cachePolicy:NSURLRequestReloadIgnoringLocalCacheData
                                            timeoutInterval:10];
        [self.webView loadRequest:request];
    }
    [self.window makeKeyAndOrderFront:nil];
    if (@available(macOS 14.0, *)) {
        [NSApp activate];
    } else {
        [NSApp activateIgnoringOtherApps:YES];
    }
}

- (void)webView:(WKWebView *)webView
decidePolicyForNavigationAction:(WKNavigationAction *)action
decisionHandler:(void (^)(WKNavigationActionPolicy))decide {
    NSURL *url = action.request.URL;
    BOOL local = [url.scheme isEqualToString:self.dashboardURL.scheme] &&
                 [url.host isEqualToString:self.dashboardURL.host] &&
                 [url.port isEqual:self.dashboardURL.port];
    decide(local ? WKNavigationActionPolicyAllow : WKNavigationActionPolicyCancel);
}

- (void)webView:(WKWebView *)webView didFinishNavigation:(WKNavigation *)navigation {
    webView.hidden = NO;
    self.message.hidden = YES;
}

- (void)showUnavailableMessage {
    self.webView.hidden = YES;
    self.message.stringValue = @"Backlog is unavailable. Choose Quick Backlog again to retry.";
    self.message.hidden = NO;
}

- (void)webView:(WKWebView *)webView didFailProvisionalNavigation:(WKNavigation *)navigation withError:(NSError *)error {
    if (error.code != NSURLErrorCancelled) {
        [self showUnavailableMessage];
    }
}

- (void)webView:(WKWebView *)webView didFailNavigation:(WKNavigation *)navigation withError:(NSError *)error {
    [self webView:webView didFailProvisionalNavigation:navigation withError:error];
}

- (void)webViewWebContentProcessDidTerminate:(WKWebView *)webView {
    [self showUnavailableMessage];
}

@end

int backlog_menu_run(const char *url) {
    if (!pthread_main_np()) {
        return 1;
    }
    @autoreleasepool {
        NSApplication *app = NSApplication.sharedApplication;
        if (![app setActivationPolicy:NSApplicationActivationPolicyAccessory]) {
            return 1;
        }
        BacklogMenu *menu = [[BacklogMenu alloc] init];
        menu.dashboardURL = [NSURL URLWithString:[NSString stringWithUTF8String:url]];
        if (menu.dashboardURL == nil || ![menu install]) {
            return 1;
        }
        [app run];
        [NSStatusBar.systemStatusBar removeStatusItem:menu.statusItem];
        [menu.window close];
        return 0;
    }
}

void backlog_menu_stop(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        [NSApp stop:nil];
        // Wake the event loop so it can observe the stop request.
        NSEvent *event = [NSEvent otherEventWithType:NSEventTypeApplicationDefined
                                          location:NSZeroPoint modifierFlags:0 timestamp:0
                                      windowNumber:0 context:nil subtype:0 data1:0 data2:0];
        [NSApp postEvent:event atStart:YES];
    });
}
