//go:build desktop && darwin

#import <Cocoa/Cocoa.h>

void configurePetNativeWindow(void) {
    // OnDomReady runs on a Go goroutine; AppKit must run on its main queue.
    dispatch_async(dispatch_get_main_queue(), ^{
        // This is the dedicated companion process, never the main client.
        // Accessory windows may accompany another application's fullscreen Space.
        [NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
        for (NSWindow *window in NSApp.windows) {
            if (![window.title isEqualToString:@"监控小猫"]) continue;

            NSWindowCollectionBehavior behavior =
                NSWindowCollectionBehaviorCanJoinAllSpaces |
                NSWindowCollectionBehaviorStationary |
                NSWindowCollectionBehaviorIgnoresCycle |
                NSWindowCollectionBehaviorFullScreenAuxiliary |
                NSWindowCollectionBehaviorFullScreenDisallowsTiling;
            // Also accompany other apps' Stage Manager sets on macOS 13+.
            if (@available(macOS 13.0, *)) {
                behavior |= NSWindowCollectionBehaviorCanJoinAllApplications;
            }
            window.collectionBehavior = behavior;
            window.level = NSFloatingWindowLevel;
            window.hidesOnDeactivate = NO;
            window.canHide = NO;
            window.opaque = NO;
            window.backgroundColor = NSColor.clearColor;
            window.hasShadow = NO;
            window.animationBehavior = NSWindowAnimationBehaviorNone;
            [window orderFrontRegardless];
        }
    });
}
