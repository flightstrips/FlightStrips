using System.Text.Json;
using System.Text.Json.Serialization;
using Microsoft.AspNetCore.Http.Connections;
using Microsoft.EntityFrameworkCore;
using Vatsim.Scandinavia.FlightStrips.Abstractions;
using Vatsim.Scandinavia.FlightStrips.Extensions;
using Vatsim.Scandinavia.FlightStrips.Host;
using Vatsim.Scandinavia.FlightStrips.Host.Controllers;
using Vatsim.Scandinavia.FlightStrips.Host.Hubs;
using Vatsim.Scandinavia.FlightStrips.Host.Middleware;
using Vatsim.Scandinavia.FlightStrips.Persistence.EfCore;

var builder = WebApplication.CreateBuilder(args);

// Add services to the container.

// Learn more about configuring Swagger/OpenAPI at https://aka.ms/aspnetcore/swashbuckle
builder.Services.ConfigureHttpJsonOptions(options =>
{
    options.SerializerOptions.Converters.Add(new JsonStringEnumConverter());
    options.SerializerOptions.PropertyNamingPolicy = JsonNamingPolicy.CamelCase;
});
builder.Services.Configure<Microsoft.AspNetCore.Mvc.JsonOptions>(options =>
{
    options.JsonSerializerOptions.Converters.Add(new JsonStringEnumConverter());
    options.JsonSerializerOptions.PropertyNamingPolicy = JsonNamingPolicy.CamelCase;
});

builder.Services.AddEndpointsApiExplorer();
builder.Services.AddSwaggerGen();
builder.Services.AddAuthorization();
builder.Services.AddFlightStripServices();
builder.Services.AddEfCore();
builder.Services.AddScoped<ITenantService, TenantService>();
builder.Services.AddScoped<IEventService, EventService>();
builder.Services.AddTransient<TenantMiddleware>();
builder.Services.AddSignalR().AddMessagePackProtocol();
builder.Services.AddCors(options =>
{
    options.AddDefaultPolicy(policyBuilder => policyBuilder.AllowAnyOrigin().AllowAnyHeader().AllowAnyMethod());
});

var connectionString = builder.Configuration.GetConnectionString("Database");

builder.Services.AddDbContext<FlightStripsDbContext>(dbBuilder => dbBuilder.UseMySql(connectionString, new MariaDbServerVersion("11.1")));

var app = builder.Build();

// Configure the HTTP request pipeline.
if (app.Environment.IsDevelopment())
{
    app.UseSwagger();
    app.UseSwaggerUI();
}

app.UseHttpsRedirection();

app.UseCors();
app.UseAuthorization();

app.UseTenantMiddleware();

app.MapHub<EventHub>("/hubs/events", options =>
{
    options.Transports = HttpTransportType.WebSockets;
});

var apiGroup = app.MapGroup("api");

apiGroup.MapStrips();
apiGroup.MapBays();
apiGroup.MapPositions();
apiGroup.MapOnlinePositions();
apiGroup.MapCoordination();

app.Run();
