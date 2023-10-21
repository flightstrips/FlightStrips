using Vatsim.Scandinavia.FlightStrips.Abstractions;

namespace Vatsim.Scandinavia.FlightStrips.Host.Middleware;

public class TenantMiddleware : IMiddleware
{
    private readonly ITenantService _tenantService;
    private readonly ILogger<TenantMiddleware> _logger;

    public TenantMiddleware(ITenantService tenantService, ILogger<TenantMiddleware> logger)
    {
        _tenantService = tenantService;
        _logger = logger;
    }

    public async Task InvokeAsync(HttpContext context, RequestDelegate next)
    {
        var routeData = context.GetRouteData();

        var airport = routeData.Values["airport"] as string;
        var session = routeData.Values["session"] as string;

        if (!string.IsNullOrEmpty(airport))
        {
            _tenantService.SetAirport(airport.ToUpperInvariant());
        }

        if (!string.IsNullOrEmpty(session))
        {
            _tenantService.SetSession(session.ToLowerInvariant());
        }

        _logger.LogInformation("Airport {Airport}, Session {Session}", airport, session);

        await next(context);
    }
}

public static class TenantMiddlewareExtensions
{
    public static IApplicationBuilder UseTenantMiddleware(this IApplicationBuilder builder)
    {
        return builder.UseMiddleware<TenantMiddleware>();
    }
}
