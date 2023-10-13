using System.ComponentModel.DataAnnotations;

using Vatsim.Scandinavia.FlightStrips.Extensions;
using Vatsim.Scandinavia.FlightStrips.Host.Controllers;
using Vatsim.Scandinavia.FlightStrips.Persistence.Redis;

var builder = WebApplication.CreateBuilder(args);

// Add services to the container.
builder.Services.Configure<RedisOptions>(builder.Configuration.GetSection(RedisOptions.Redis));
var redisOptions = builder.Configuration.GetSection(RedisOptions.Redis).Get<RedisOptions>();
if (redisOptions is null)
{
    throw new ValidationException();
}

// Learn more about configuring Swagger/OpenAPI at https://aka.ms/aspnetcore/swashbuckle
builder.Services.AddEndpointsApiExplorer();
builder.Services.AddSwaggerGen();
builder.Services.AddAuthorization();
builder.Services.AddFlightStripServices();
builder.Services.AddRedisStorage(redisOptions);

var app = builder.Build();

// Configure the HTTP request pipeline.
if (app.Environment.IsDevelopment())
{
    app.UseSwagger();
    app.UseSwaggerUI();
}

app.UseHttpsRedirection();

app.UseAuthorization();

app.MapStrips();
app.MapBays();

app.Run();
