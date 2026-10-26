using System.Drawing.Drawing2D;
using System.IO.Pipes;
using Microsoft.Win32.SafeHandles;
using System.Runtime.InteropServices;
using System.Security.Principal;
using System.Text;
using System.Text.Json;
using System.Diagnostics;
using Microsoft.Web.WebView2.Core;
using Microsoft.Web.WebView2.WinForms;

namespace TranscoderD.Desktop;

internal static class Program
{
    [STAThread]
    private static void Main(string[] args)
    {
        ApplicationConfiguration.Initialize();
        string sid = WindowsIdentity.GetCurrent().User?.Value ?? throw new InvalidOperationException("Could not identify the current user.");
        using var mutex = new Mutex(true, @"Local\TranscoderD.Tray." + sid, out bool created);
        using var showEvent = new EventWaitHandle(false, EventResetMode.AutoReset, @"Local\TranscoderD.Show." + sid);
        using var quitEvent = new EventWaitHandle(false, EventResetMode.AutoReset, @"Local\TranscoderD.Quit." + sid);
        if (!created) { if (args.Contains("--quit")) quitEvent.Set(); else showEvent.Set(); return; }
        if (args.Contains("--quit")) return;
        bool rejectSmoke = args.Contains("--smoke-reject");
        bool smoke = args.Contains("--smoke") || rejectSmoke;
        using var form = new MainForm(sid, args.Contains("--background") || smoke, smoke, rejectSmoke);
        _ = Task.Run(() => { while (showEvent.WaitOne()) { if (form.IsDisposed) break; try { form.BeginInvoke((Action)form.ShowWindow); } catch (InvalidOperationException) { break; } } });
        _ = Task.Run(() => { quitEvent.WaitOne(); if (!form.IsDisposed) { try { form.BeginInvoke((Action)form.Quit); } catch (InvalidOperationException) { } } });
        Application.Run(form);
    }
}

internal sealed class PipeClient(string sid, bool allowUserOwner)
{
    [DllImport("advapi32.dll", SetLastError = true)] private static extern uint GetSecurityInfo(SafePipeHandle handle, int type, uint information, out IntPtr owner, out IntPtr group, out IntPtr dacl, out IntPtr sacl, out IntPtr descriptor);
    [DllImport("advapi32.dll", CharSet = CharSet.Unicode, SetLastError = true)] private static extern bool ConvertSidToStringSid(IntPtr sid, out IntPtr text);
    [DllImport("kernel32.dll")] private static extern IntPtr LocalFree(IntPtr value);
    private void VerifyOwner(SafePipeHandle handle)
    {
        uint error = GetSecurityInfo(handle, 6, 1, out IntPtr owner, out _, out _, out _, out IntPtr descriptor);
        if (error != 0) throw new IOException("Could not verify the service identity.");
        try {
            if (!ConvertSidToStringSid(owner, out IntPtr text)) throw new IOException("Could not verify the service identity.");
            try {
                string actual = Marshal.PtrToStringUni(text) ?? "";
                if (!string.Equals(actual, "S-1-5-18", StringComparison.OrdinalIgnoreCase) && !(allowUserOwner && string.Equals(actual, sid, StringComparison.OrdinalIgnoreCase))) throw new IOException("The service identity is invalid.");
            } finally { LocalFree(text); }
        } finally { LocalFree(descriptor); }
    }
    public async Task<string> SendAsync(string request)
    {
        using var timeout = new CancellationTokenSource(TimeSpan.FromSeconds(12));
        using var pipe = new NamedPipeClientStream(".", "transcoderd-desktop-" + sid, PipeDirection.InOut, PipeOptions.Asynchronous);
        await pipe.ConnectAsync(timeout.Token);
        VerifyOwner(pipe.SafePipeHandle);
        await using var writer = new StreamWriter(pipe, new UTF8Encoding(false), leaveOpen:true) { AutoFlush = true };
        using var reader = new StreamReader(pipe, Encoding.UTF8, false, 1024, true);
        await writer.WriteLineAsync(request.AsMemory(), timeout.Token);
        return await reader.ReadLineAsync(timeout.Token) ?? throw new IOException("The service returned an empty response.");
    }
}

internal sealed class MainForm : Form
{
    private const string Host = "app.transcoderd.local";
    private readonly PipeClient client;
    private readonly NotifyIcon tray;
    private readonly WebView2 browser = new() { Dock = DockStyle.Fill };
    private readonly bool smoke;
    private readonly bool rejectSmoke;
    private bool quitting;
    private int lastTarget = 2;

    public MainForm(string sid, bool background, bool smoke, bool rejectSmoke)
    {
        this.smoke = smoke;
        this.rejectSmoke = rejectSmoke;
        client = new PipeClient(sid, smoke && !rejectSmoke);
        Text = "TranscoderD"; Size = new Size(1180, 780); MinimumSize = new Size(800, 600);
        StartPosition = FormStartPosition.CenterScreen; BackColor = Color.FromArgb(244, 246, 250); Icon = MakeIcon();
        Controls.Add(browser);
        var menu = new ContextMenuStrip();
        menu.Items.Add("Open TranscoderD", null, (_, _) => ShowWindow());
        menu.Items.Add("Pause new jobs", null, async (_, _) => await SetLimit(0));
        menu.Items.Add("Resume", null, async (_, _) => await SetLimit(lastTarget));
        menu.Items.Add(new ToolStripSeparator());
        menu.Items.Add("Exit tray", null, (_, _) => { quitting = true; Close(); });
        tray = new NotifyIcon { Icon = Icon, Text = "TranscoderD", ContextMenuStrip = menu, Visible = true };
        tray.DoubleClick += (_, _) => ShowWindow();
        FormClosing += (_, e) => { if (!quitting && e.CloseReason == CloseReason.UserClosing) { e.Cancel = true; Hide(); } };
        FormClosed += (_, _) => { tray.Visible = false; tray.Dispose(); browser.Dispose(); };
        Shown += async (_, _) => { if (background) Hide(); await InitializeBrowser(); };
    }
    public void Quit() { quitting = true; Close(); }
    public void ShowWindow() { Show(); WindowState = FormWindowState.Normal; Activate(); }
    private async Task InitializeBrowser()
    {
        try {
            string data = Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData), "TranscoderD", "WebView2");
            var environment = await CoreWebView2Environment.CreateAsync(userDataFolder:data);
            await browser.EnsureCoreWebView2Async(environment);
            var core = browser.CoreWebView2;
            core.Settings.AreDevToolsEnabled = false; core.Settings.AreHostObjectsAllowed = false;
            core.Settings.IsPasswordAutosaveEnabled = false; core.Settings.IsGeneralAutofillEnabled = false;
            core.SetVirtualHostNameToFolderMapping(Host, Path.Combine(AppContext.BaseDirectory, "wwwroot"), CoreWebView2HostResourceAccessKind.DenyCors);
            core.NavigationStarting += (_, e) => { if (!IsLocal(e.Uri)) e.Cancel = true; };
            core.NewWindowRequested += (_, e) => e.Handled = true;
            core.PermissionRequested += (_, e) => e.State = CoreWebView2PermissionState.Deny;
            core.WebMessageReceived += async (_, e) => await HandleMessage(e);
            if (smoke) core.NavigationCompleted += async (_, e) => {
                try {
                    bool ok = e.IsSuccess;
                    for (int i = 0; ok && i < 80; i++) {
                        string state = rejectSmoke ? "error" : "online";
                        string result = await core.ExecuteScriptAsync($"Boolean(document.getElementById('settings-form') && document.getElementById('job-list') && document.getElementById('connection').classList.contains('{state}'))");
                        if (result == "true") { Environment.ExitCode = 0; Quit(); return; }
                        await Task.Delay(250);
                    }
                    string details = await core.ExecuteScriptAsync("JSON.stringify({ready:document.readyState,connection:document.getElementById('connection')?.className,text:document.getElementById('connection')?.innerText,serviceError:window.__lastServiceError,settings:!!document.getElementById('settings-form'),jobs:!!document.getElementById('job-list')})");
                    FailSmoke($"Navigation success: {e.IsSuccess}; web error: {e.WebErrorStatus}; browser: {core.Environment.BrowserVersionString}; document: {details}");
                } catch (Exception ex) { FailSmoke(ex.ToString()); }
            };
            core.Navigate("https://" + Host + "/index.html");
        } catch (Exception ex) { if (smoke) { FailSmoke(ex.ToString()); return; } ShowWindow(); MessageBox.Show("Could not start the web interface. Make sure Microsoft Edge WebView2 Runtime is installed.\n\n" + ex.Message, "TranscoderD", MessageBoxButtons.OK, MessageBoxIcon.Error); }
    }
    private void FailSmoke(string details)
    {
        try { File.WriteAllText(Path.Combine(Path.GetTempPath(), $"TranscoderD-tray-smoke-{Environment.ProcessId}.log"), details); } catch { }
        Environment.ExitCode = 1; Quit();
    }
    private static bool IsLocal(string value) => Uri.TryCreate(value, UriKind.Absolute, out var uri) && uri.Scheme == "https" && uri.Host == Host && uri.Port == 443;
    private async Task HandleMessage(CoreWebView2WebMessageReceivedEventArgs e)
    {
        if (!IsLocal(e.Source) || e.WebMessageAsJson.Length > 65536) return;
        long id = 0;
        try {
            using var doc = JsonDocument.Parse(e.WebMessageAsJson);
            var root = doc.RootElement; id = root.GetProperty("id").GetInt64();
            string command = root.GetProperty("command").GetString() ?? "";
            if (command == "show-window") { if (!smoke) ShowWindow(); Reply(id, true, "{}"); return; }
            if (command == "start-service") { StartService(); Reply(id, true, "{}"); return; }
            if (command is not ("status" or "configure" or "set-max-jobs")) throw new InvalidOperationException("Command not allowed");
            string response = await client.SendAsync(root.GetRawText());
            using var result = JsonDocument.Parse(response);
            if (command == "status" && result.RootElement.TryGetProperty("status", out var status) && status.TryGetProperty("worker", out var worker) && worker.ValueKind == JsonValueKind.Object && worker.TryGetProperty("maxActiveJobs", out var target) && target.TryGetInt32(out int n) && n > 0) lastTarget = n;
            Reply(id, true, result.RootElement.GetRawText());
        } catch (Exception ex) { Reply(id, false, JsonSerializer.Serialize(ex.Message)); }
    }
    private void Reply(long id, bool ok, string payload)
    {
        if (browser.IsDisposed || browser.CoreWebView2 == null) return;
        browser.CoreWebView2.PostWebMessageAsJson($"{{\"id\":{id},\"ok\":{(ok ? "true" : "false")},\"payload\":{payload}}}");
    }
    private async Task SetLimit(int value)
    {
        try { var response = await client.SendAsync(JsonSerializer.Serialize(new { command = "set-max-jobs", maxJobs = value })); using var doc = JsonDocument.Parse(response); if (doc.RootElement.TryGetProperty("error", out var error) && error.GetString() is { Length: > 0 } text) throw new IOException(text); }
        catch { ShowWindow(); }
    }
    private static void StartService()
    {
        string service = Path.Combine(AppContext.BaseDirectory, "transcoderd-desktop-service.exe");
        Process.Start(new ProcessStartInfo(service, "--start") { UseShellExecute = true, Verb = "runas" });
    }
    [DllImport("user32.dll")] private static extern bool DestroyIcon(IntPtr handle);
    private static Icon MakeIcon()
    {
        using var bitmap = new Bitmap(32, 32); using var g = Graphics.FromImage(bitmap); g.SmoothingMode = SmoothingMode.AntiAlias;
        g.Clear(Color.Transparent); using var brush = new SolidBrush(Color.FromArgb(68, 107, 246)); g.FillEllipse(brush, 1, 1, 30, 30);
        using var font = new Font("Segoe UI", 17, FontStyle.Bold, GraphicsUnit.Pixel); using var white = new SolidBrush(Color.White);
        g.DrawString("T", font, white, 8, 4); IntPtr handle = bitmap.GetHicon(); using var native = Icon.FromHandle(handle); var copy = (Icon)native.Clone(); DestroyIcon(handle); return copy;
    }
}
