// Build with the production window adapter, then run in a macOS GUI session:
// clang -fblocks -framework Cocoa scripts/test-pet-window-darwin.m \
//   cmd/aissh-desktop/pet_window_darwin.m -o .cache/test-pet-window
#import <Cocoa/Cocoa.h>
#include <assert.h>

void configurePetNativeWindow(void);

int main(void) {
    @autoreleasepool {
        [NSApplication sharedApplication];
        [NSApp setActivationPolicy:NSApplicationActivationPolicyRegular];
        NSWindow *pet = [[NSWindow alloc] initWithContentRect:NSMakeRect(40, 40, 320, 330)
            styleMask:NSWindowStyleMaskBorderless backing:NSBackingStoreBuffered defer:NO];
        pet.title = @"监控小猫";
        NSWindow *other = [[NSWindow alloc] initWithContentRect:NSMakeRect(40, 40, 100, 100)
            styleMask:NSWindowStyleMaskTitled backing:NSBackingStoreBuffered defer:NO];
        other.title = @"Unrelated window";
        NSWindowCollectionBehavior originalBehavior = other.collectionBehavior;

        configurePetNativeWindow();
        dispatch_async(dispatch_get_main_queue(), ^{
            assert(NSApp.activationPolicy == NSApplicationActivationPolicyAccessory);
            assert(pet.level == NSFloatingWindowLevel);
            assert(pet.collectionBehavior & NSWindowCollectionBehaviorCanJoinAllSpaces);
            assert(pet.collectionBehavior & NSWindowCollectionBehaviorFullScreenAuxiliary);
            assert(pet.collectionBehavior & NSWindowCollectionBehaviorStationary);
            assert(!(pet.collectionBehavior & NSWindowCollectionBehaviorFullScreenPrimary));
            assert(!(pet.collectionBehavior & NSWindowCollectionBehaviorMoveToActiveSpace));
            if (@available(macOS 13.0, *)) {
                assert(pet.collectionBehavior & NSWindowCollectionBehaviorCanJoinAllApplications);
            }
            assert(!pet.hidesOnDeactivate && !pet.canHide);
            assert(!pet.opaque && !pet.hasShadow);
            assert(pet.backgroundColor.alphaComponent == 0);
            assert(pet.visible);
            assert(other.level == NSNormalWindowLevel);
            assert(other.collectionBehavior == originalBehavior);
            puts("PASS: native pet window level, Spaces/fullscreen/Stage Manager flags, transparency, visibility and unrelated window isolation");
            exit(0);
        });
        [NSApp run];
    }
}
