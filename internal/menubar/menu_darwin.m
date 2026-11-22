#import <AppKit/AppKit.h>
#include <pthread.h>
#include <stdint.h>
#include <stdlib.h>

extern char *backlogMenuSave(uintptr_t handle, char *url, char *username);
extern void backlogMenuVersion(uintptr_t handle, char *version);

@interface BacklogMenu : NSObject <NSMenuDelegate, NSURLSessionTaskDelegate>
@property (strong) NSURL *dashboardURL;
@property (copy) NSString *username;
@property uintptr_t handle;
@property (strong) NSStatusItem *statusItem;
@property (strong) NSURLSession *session;
@property (strong) NSURLSessionDataTask *pollTask;
@property (strong) NSTimer *pollTimer;
@property (copy) NSArray<NSDictionary *> *projects;
@property BOOL connected;
@property BOOL polled;
@property NSUInteger generation;
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
    menu.delegate = self;
    menu.autoenablesItems = NO;
    self.statusItem.menu = menu;
    NSURLSessionConfiguration *config = NSURLSessionConfiguration.ephemeralSessionConfiguration;
    config.timeoutIntervalForRequest = 4;
    config.timeoutIntervalForResource = 5;
    self.session = [NSURLSession sessionWithConfiguration:config delegate:self delegateQueue:nil];
    __weak BacklogMenu *weakSelf = self;
    self.pollTimer = [NSTimer timerWithTimeInterval:10 repeats:YES block:^(NSTimer *timer) { [weakSelf poll]; }];
    [NSRunLoop.mainRunLoop addTimer:self.pollTimer forMode:NSRunLoopCommonModes];
    return YES;
}

- (void)poll {
    if (self.pollTask != nil || self.dashboardURL == nil) return;
    NSURL *endpoint = [[self.dashboardURL URLByAppendingPathComponent:@"api"] URLByAppendingPathComponent:@"client"];
    NSMutableURLRequest *request = [NSMutableURLRequest requestWithURL:endpoint];
    request.HTTPMethod = @"POST";
    [request setValue:@"application/json" forHTTPHeaderField:@"Content-Type"];
    request.HTTPBody = [NSJSONSerialization dataWithJSONObject:@{ @"username": self.username ?: @"" } options:0 error:nil];
    NSUInteger generation = self.generation;
    __weak BacklogMenu *weakSelf = self;
    self.pollTask = [self.session dataTaskWithRequest:request completionHandler:^(NSData *data, NSURLResponse *response, NSError *error) {
        NSDictionary *result = nil;
        if (error == nil && [response isKindOfClass:NSHTTPURLResponse.class] && ((NSHTTPURLResponse *)response).statusCode == 200 && data != nil) {
            id decoded = [NSJSONSerialization JSONObjectWithData:data options:0 error:nil];
            if ([decoded isKindOfClass:NSDictionary.class]) result = decoded;
        }
        dispatch_async(dispatch_get_main_queue(), ^{
            BacklogMenu *menu = weakSelf;
            if (menu == nil || menu.generation != generation) return;
            menu.pollTask = nil;
            menu.polled = YES;
            NSString *serverVersion = [result[@"server_version"] isKindOfClass:NSString.class] ? result[@"server_version"] : @"";
            NSArray *projects = [result[@"projects"] isKindOfClass:NSArray.class] ? result[@"projects"] : nil;
            menu.connected = projects != nil;
            if (menu.connected) menu.projects = projects;
            backlogMenuVersion(menu.handle, (char *)serverVersion.UTF8String);
        });
    }];
    [self.pollTask resume];
}

- (void)URLSession:(NSURLSession *)session task:(NSURLSessionTask *)task
    willPerformHTTPRedirection:(NSHTTPURLResponse *)response newRequest:(NSURLRequest *)request
    completionHandler:(void (^)(NSURLRequest *))complete {
    complete(nil);
}

- (NSMenuItem *)add:(NSString *)title action:(SEL)action toMenu:(NSMenu *)menu {
    NSMenuItem *item = [menu addItemWithTitle:title action:action keyEquivalent:@""];
    item.target = self;
    return item;
}

- (NSInteger)countForKey:(NSString *)key inProject:(NSDictionary *)project {
    id value = project[key];
    return [value isKindOfClass:NSNumber.class] ? [value integerValue] : 0;
}

- (NSMenuItem *)projectItem:(NSDictionary *)project {
    NSString *name = project[@"name"];
    NSInteger todo = [self countForKey:@"todo" inProject:project];
    NSInteger doing = [self countForKey:@"doing" inProject:project];
    NSInteger blocked = [self countForKey:@"blocked" inProject:project];
    NSInteger done = [self countForKey:@"done" inProject:project];
    NSInteger rejected = [self countForKey:@"rejected" inProject:project];
    NSString *counts = [NSString stringWithFormat:@"To do %ld · Doing %ld · Blocked %ld · Done %ld",
        (long)todo, (long)doing, (long)blocked, (long)done];
    if (rejected > 0) counts = [counts stringByAppendingFormat:@" · Rejected %ld", (long)rejected];

    NSColor *color = NSColor.systemGrayColor;
    NSString *status = self.connected ? @"Inactive" : @"Disconnected";
    if (self.connected && blocked > 0) {
        color = NSColor.systemRedColor;
        status = @"Needs attention";
    } else if (self.connected && doing > 0) {
        color = NSColor.systemGreenColor;
        status = @"Active";
    }
    NSImage *dot = [NSImage imageWithSize:NSMakeSize(10, 10) flipped:NO drawingHandler:^BOOL(NSRect frame) {
        [color setFill];
        [[NSBezierPath bezierPathWithOvalInRect:NSInsetRect(frame, 1, 1)] fill];
        return YES;
    }];
    NSImageView *indicator = [NSImageView imageViewWithImage:dot];
    [indicator setAccessibilityElement:NO];

    NSTextField *projectName = [NSTextField labelWithString:name];
    projectName.font = [NSFont systemFontOfSize:13 weight:NSFontWeightSemibold];
    projectName.lineBreakMode = NSLineBreakByTruncatingTail;
    NSTextField *summary = [NSTextField labelWithString:counts];
    summary.font = [NSFont monospacedDigitSystemFontOfSize:12 weight:NSFontWeightRegular];
    summary.textColor = NSColor.secondaryLabelColor;
    CGFloat nameWidth = MIN(240, ceil(projectName.intrinsicContentSize.width));
    CGFloat summaryWidth = ceil(summary.intrinsicContentSize.width);
    NSView *row = [[NSView alloc] initWithFrame:NSMakeRect(0, 0, 58 + nameWidth + summaryWidth, 25)];
    indicator.frame = NSMakeRect(14, 8, 10, 10);
    projectName.frame = NSMakeRect(32, 4, nameWidth, 17);
    summary.frame = NSMakeRect(42 + nameWidth, 4, summaryWidth, 17);
    for (NSView *view in @[indicator, projectName, summary]) [row addSubview:view];
    NSString *description = [NSString stringWithFormat:@"%@, %@, %@", name, status, counts];
    [row setAccessibilityElement:YES];
    [row setAccessibilityRole:NSAccessibilityStaticTextRole];
    [row setAccessibilityLabel:description];
    row.toolTip = description;
    NSMenuItem *item = [[NSMenuItem alloc] initWithTitle:description action:NULL keyEquivalent:@""];
    item.view = row;
    return item;
}

- (void)menuNeedsUpdate:(NSMenu *)menu {
    [menu removeAllItems];
    if (!self.connected || self.projects.count == 0) {
        NSString *message = self.dashboardURL == nil ? @"Set a Backlog URL" :
            (self.connected ? @"No projects" : (self.polled ? @"Disconnected" : @"Connecting…"));
        [self add:message action:NULL toMenu:menu].enabled = NO;
    }
    for (id value in self.projects) {
        if (![value isKindOfClass:NSDictionary.class]) continue;
        NSDictionary *project = value;
        if (![project[@"name"] isKindOfClass:NSString.class]) continue;
        [menu addItem:[self projectItem:project]];
    }
    [menu addItem:NSMenuItem.separatorItem];
    [self add:@"Settings…" action:@selector(showSettings:) toMenu:menu];
    [self add:@"Quit Backlog" action:@selector(quit:) toMenu:menu];
}

- (void)showSettings:(id)sender {
    NSAlert *alert = [[NSAlert alloc] init];
    alert.messageText = @"Backlog Settings";
    [alert addButtonWithTitle:@"Save"];
    [alert addButtonWithTitle:@"Cancel"];
    NSView *fields = [[NSView alloc] initWithFrame:NSMakeRect(0, 0, 350, 105)];
    NSTextField *urlLabel = [NSTextField labelWithString:@"Backlog URL"];
    urlLabel.frame = NSMakeRect(0, 84, 350, 18);
    NSTextField *url = [[NSTextField alloc] initWithFrame:NSMakeRect(0, 60, 350, 24)];
    url.placeholderString = @"https://backlog.<tailnet>.ts.net";
    url.stringValue = self.dashboardURL.absoluteString ?: @"";
    [url setAccessibilityLabel:urlLabel.stringValue];
    NSTextField *nameLabel = [NSTextField labelWithString:@"Username (optional)"];
    nameLabel.frame = NSMakeRect(0, 31, 350, 18);
    NSTextField *name = [[NSTextField alloc] initWithFrame:NSMakeRect(0, 6, 350, 24)];
    name.stringValue = self.username ?: @"";
    [name setAccessibilityLabel:nameLabel.stringValue];
    url.nextKeyView = name;
    for (NSView *field in @[urlLabel, url, nameLabel, name]) [fields addSubview:field];
    alert.accessoryView = fields;
    alert.window.initialFirstResponder = url;
    [self activate];
    while ([alert runModal] == NSAlertFirstButtonReturn) {
        NSCharacterSet *space = NSCharacterSet.whitespaceAndNewlineCharacterSet;
        NSString *address = [url.stringValue stringByTrimmingCharactersInSet:space];
        NSString *username = [name.stringValue stringByTrimmingCharactersInSet:space];
        NSURL *nextURL = [NSURL URLWithString:address];
        char *error = nextURL == nil ? NULL : backlogMenuSave(self.handle, (char *)address.UTF8String, (char *)username.UTF8String);
        if (nextURL == nil || error != NULL) {
            alert.informativeText = error == NULL ? @"Enter an HTTP or HTTPS dashboard URL." : [NSString stringWithUTF8String:error];
            free(error);
            continue;
        }
        if (![nextURL isEqual:self.dashboardURL]) self.projects = nil;
        self.dashboardURL = nextURL;
        self.username = username;
        self.generation++;
        [self.pollTask cancel];
        self.pollTask = nil;
        self.polled = NO;
        self.connected = NO;
        [self poll];
        return;
    }
}

- (void)activate {
    if (@available(macOS 14.0, *)) {
        [NSApp activate];
    } else {
        [NSApp activateIgnoringOtherApps:YES];
    }
}

- (void)quit:(id)sender {
    [NSApp stop:nil];
}

- (void)close {
    self.generation++;
    [self.pollTimer invalidate];
    [self.session invalidateAndCancel];
    [NSStatusBar.systemStatusBar removeStatusItem:self.statusItem];
}

@end

int backlog_menu_run(const char *url, const char *username, int showSettings, uintptr_t handle) {
    if (!pthread_main_np()) {
        return 1;
    }
    @autoreleasepool {
        NSApplication *app = NSApplication.sharedApplication;
        if (![app setActivationPolicy:NSApplicationActivationPolicyAccessory]) {
            return 1;
        }
        BacklogMenu *menu = [[BacklogMenu alloc] init];
        NSURL *initial = [NSURL URLWithString:[NSString stringWithUTF8String:url]];
        if (([initial.scheme isEqualToString:@"http"] || [initial.scheme isEqualToString:@"https"]) && initial.host.length != 0) {
            menu.dashboardURL = initial;
        }
        menu.username = [NSString stringWithUTF8String:username];
        menu.handle = handle;
        if (![menu install]) {
            return 1;
        }
        [menu poll];
        if (showSettings) dispatch_async(dispatch_get_main_queue(), ^{ [menu showSettings:nil]; });
        [app run];
        [menu close];
        return 0;
    }
}

void backlog_menu_stop(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (NSApp.modalWindow != nil) [NSApp abortModal];
        [NSApp stop:nil];
        // Wake the event loop so it can observe the stop request.
        NSEvent *event = [NSEvent otherEventWithType:NSEventTypeApplicationDefined
                                          location:NSZeroPoint modifierFlags:0 timestamp:0
                                      windowNumber:0 context:nil subtype:0 data1:0 data2:0];
        [NSApp postEvent:event atStart:YES];
    });
}
