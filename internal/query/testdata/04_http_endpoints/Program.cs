var app = WebApplication.Create();

app.MapGet("/api/health", () => "ok");
app.MapDelete("/api/cache/{key}", (string key) => key);

app.Run();
