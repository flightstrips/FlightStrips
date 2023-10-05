using Vatsim.Scandinavia.FlightStrips.Abstractions.Interfaces;
using Vatsim.Scandinavia.FlightStrips.Host.Controllers;
using Vatsim.Scandinavia.FlightStrips.Services;

var builder = WebApplication.CreateBuilder(args);

// Add services to the container.

// Learn more about configuring Swagger/OpenAPI at https://aka.ms/aspnetcore/swashbuckle
builder.Services.AddEndpointsApiExplorer();
builder.Services.AddSwaggerGen();
builder.Services.AddAuthorization();
builder.Services.AddScoped<IStripService, StripService>();

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

app.Run();
