import Cocoa
import Foundation
import WebKit

final class IMAJERAppDelegate: NSObject, NSApplicationDelegate, NSWindowDelegate, WKNavigationDelegate {
    private let serviceURL = URL(string: "http://127.0.0.1:8765")!
    private var backend: Process?
    private var window: NSWindow?
    private var terminating = false

    func applicationDidFinishLaunching(_ notification: Notification) {
        configureMenu()
        do {
            try startBackend()
            waitForBackend(attempt: 0)
        } catch {
            showFatalError("IMAJER motoru başlatılamadı.\n\n\(error.localizedDescription)")
        }
    }

    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool {
        true
    }

    func applicationShouldTerminate(_ sender: NSApplication) -> NSApplication.TerminateReply {
        guard !terminating else {
            return .terminateNow
        }
        terminating = true
        guard let backend, backend.isRunning else {
            return .terminateNow
        }
        backend.terminationHandler = { _ in
            DispatchQueue.main.async {
                sender.reply(toApplicationShouldTerminate: true)
            }
        }
        backend.terminate()
        DispatchQueue.main.asyncAfter(deadline: .now() + 7) {
            if backend.isRunning {
                backend.interrupt()
            }
        }
        return .terminateLater
    }

    private func startBackend() throws {
        guard let executableDirectory = Bundle.main.executableURL?.deletingLastPathComponent() else {
            throw AppError("Uygulama dizini bulunamadı.")
        }
        let backendURL = executableDirectory.appendingPathComponent("imajer-core")
        guard FileManager.default.isExecutableFile(atPath: backendURL.path) else {
            throw AppError("Paket içindeki imajer-core bulunamadı.")
        }

        let process = Process()
        process.executableURL = backendURL
        process.arguments = ["ui", "--listen", "127.0.0.1:8765", "--no-open"]
        process.standardOutput = FileHandle.nullDevice
        process.standardError = FileHandle.nullDevice
        process.terminationHandler = { [weak self] process in
            DispatchQueue.main.async {
                guard let self, !self.terminating else {
                    return
                }
                if process.terminationStatus == 0 {
                    self.terminating = true
                    NSApp.terminate(nil)
                    return
                }
                self.showFatalError("IMAJER motoru beklenmedik biçimde kapandı (kod \(process.terminationStatus)).")
            }
        }
        try process.run()
        backend = process
    }

    private func waitForBackend(attempt: Int) {
        guard attempt < 80 else {
            showFatalError("IMAJER yerel servisi zamanında hazır olmadı.")
            return
        }
        var request = URLRequest(url: serviceURL.appendingPathComponent("api/health"))
        request.timeoutInterval = 0.5
        URLSession.shared.dataTask(with: request) { [weak self] _, response, _ in
            let ready = (response as? HTTPURLResponse)?.statusCode == 200
            DispatchQueue.main.async {
                guard let self else {
                    return
                }
                if ready {
                    self.showMainWindow()
                } else {
                    DispatchQueue.main.asyncAfter(deadline: .now() + 0.15) {
                        self.waitForBackend(attempt: attempt + 1)
                    }
                }
            }
        }.resume()
    }

    private func showMainWindow() {
        guard window == nil else {
            window?.makeKeyAndOrderFront(nil)
            NSApp.activate(ignoringOtherApps: true)
            return
        }

        let configuration = WKWebViewConfiguration()
        configuration.websiteDataStore = .nonPersistent()
        let webView = WKWebView(frame: .zero, configuration: configuration)
        webView.navigationDelegate = self

        let frame = NSRect(x: 0, y: 0, width: 1240, height: 820)
        let applicationWindow = NSWindow(
            contentRect: frame,
            styleMask: [.titled, .closable, .miniaturizable, .resizable],
            backing: .buffered,
            defer: false
        )
        applicationWindow.title = "IMAJER — Adli İmaj Alma"
        applicationWindow.minSize = NSSize(width: 980, height: 680)
        applicationWindow.contentView = webView
        applicationWindow.delegate = self
        applicationWindow.center()
        applicationWindow.makeKeyAndOrderFront(nil)
        window = applicationWindow

        webView.load(URLRequest(url: serviceURL))
        NSApp.activate(ignoringOtherApps: true)
    }

    private func configureMenu() {
        let mainMenu = NSMenu()
        let applicationItem = NSMenuItem()
        mainMenu.addItem(applicationItem)

        let applicationMenu = NSMenu()
        applicationMenu.addItem(
            withTitle: "IMAJER Hakkında",
            action: #selector(NSApplication.orderFrontStandardAboutPanel(_:)),
            keyEquivalent: ""
        )
        applicationMenu.addItem(.separator())
        let quitItem = NSMenuItem(
            title: "IMAJER Uygulamasından Çık",
            action: #selector(NSApplication.terminate(_:)),
            keyEquivalent: "q"
        )
        applicationMenu.addItem(quitItem)
        applicationItem.submenu = applicationMenu
        NSApp.mainMenu = mainMenu
    }

    private func showFatalError(_ message: String) {
        guard !terminating else {
            return
        }
        let alert = NSAlert()
        alert.alertStyle = .critical
        alert.messageText = "IMAJER açılamadı"
        alert.informativeText = message
        alert.addButton(withTitle: "Kapat")
        alert.runModal()
        NSApp.terminate(nil)
    }
}

struct AppError: LocalizedError {
    let message: String

    init(_ message: String) {
        self.message = message
    }

    var errorDescription: String? {
        message
    }
}

let application = NSApplication.shared
let delegate = IMAJERAppDelegate()
application.setActivationPolicy(.regular)
application.delegate = delegate
application.run()
