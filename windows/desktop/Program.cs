using System.Drawing.Drawing2D;
using System.IO.Pipes;
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
        string sid = WindowsIdentity.GetCurrent().User?.Value ?? throw new InvalidOperationException("No se pudo identificar al usuario.");
        using var mutex = new Mutex(true, @"Local\TranscoderD.Tray." + sid, out bool created);
        using var showEvent = new EventWaitHandle(false, EventResetMode.AutoReset, @"Local\TranscoderD.Show." + sid);
        using var quitEvent = new EventWaitHandle(false, EventResetMode.AutoReset, @"Local\TranscoderD.Quit." + sid);
        if (!created) { if (args.Contains("--quit")) quitEvent.Set(); else showEvent.Set(); return; }
        if (args.Contains("--quit")) return;
        using var form = new MainForm(sid, args.Contains("--background") || args.Contains("--smoke"), args.Contains("--smoke"));
        _ = Task.Run(() => { while (showEvent.WaitOne()) { if (form.IsDisposed) break; try { form.BeginInvoke((Action)form.ShowWindow); } catch (InvalidOperationException) { break; } } });
        _ = Task.Run(() => { quitEvent.WaitOne(); if (!form.IsDisposed) { try { form.BeginInvoke((Action)form.Quit); } catch (InvalidOperationException) { } } });
        Application.Run(form);
    }
}

internal sealed class PipeClient(string sid)
{
    public async Task<string> SendAsync(string request)
    {
        using var timeout = new CancellationTokenSource(TimeSpan.FromSeconds(12));
        using var pipe = new NamedPipeClientStream(".", "transcoderd-desktop-" + sid, PipeDirection.InOut, PipeOptions.Asynchronous);
        await pipe.ConnectAsync(timeout.Token);
        await using var writer = new StreamWriter(pipe, new UTF8Encoding(false), leaveOpen:true) { AutoFlush = true };
        using var reader = new StreamReader(pipe, Encoding.UTF8, false, 1024, true);
        await writer.WriteLineAsync(request.AsMemory(), timeout.Token);
        return await reader.ReadLineAsync(timeout.Token) ?? throw new IOException("Respuesta vacía del servicio.");
    }
}

internal sealed class MainForm : Form
{
    private const string Host = "app.transcoderd.local";
    private readonly PipeClient client;
    private readonly NotifyIcon tray;
    private readonly WebView2 browser = new() { Dock = DockStyle.Fill };
    private readonly bool smoke;
    private bool quitting;
    private int lastTarget = 2;

    public MainForm(string sid, bool background, bool smoke)
    {
        this.smoke = smoke;
        client = new PipeClient(sid);
        Text = "TranscoderD"; Size = new Size(1180, 780); MinimumSize = new Size(800, 600);
        StartPosition = FormStartPosition.CenterScreen; BackColor = Color.FromArgb(244, 246, 250); Icon = MakeIcon();
        Controls.Add(browser);
        var menu = new ContextMenuStrip();
        menu.Items.Add("Abrir TranscoderD", null, (_, _) => ShowWindow());
        menu.Items.Add("Pausar nuevos trabajos", null, async (_, _) => await SetLimit(0));
        menu.Items.Add("Reanudar", null, async (_, _) => await SetLimit(lastTarget));
        menu.Items.Add(new ToolStripSeparator());
        menu.Items.Add("Salir de la bandeja", null, (_, _) => { quitting = true; Close(); });
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
                bool ok = e.IsSuccess;
                for (int i = 0; ok && i < 80; i++) {
                    string result = await core.ExecuteScriptAsync("Boolean(document.getElementById('settings-form') && document.getElementById('job-list') && document.getElementById('connection').classList.contains('online'))");
                    if (result == "true") { Environment.ExitCode = 0; Quit(); return; }
                    await Task.Delay(250);
                }
                Environment.ExitCode = 1; Quit();
            };
            core.Navigate("https://" + Host + "/index.html");
        } catch (Exception ex) { if (smoke) { Environment.ExitCode = 1; Quit(); return; } ShowWindow(); MessageBox.Show("No se pudo iniciar la interfaz web. Comprueba que Microsoft Edge WebView2 Runtime esté instalado.\n\n" + ex.Message, "TranscoderD", MessageBoxButtons.OK, MessageBoxIcon.Error); }
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
            if (command is not ("status" or "configure" or "set-max-jobs")) throw new InvalidOperationException("Comando no permitido");
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
