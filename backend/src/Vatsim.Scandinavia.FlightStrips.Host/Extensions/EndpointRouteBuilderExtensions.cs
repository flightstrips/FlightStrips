using System.Diagnostics.CodeAnalysis;
using System.Text.RegularExpressions;
using Microsoft.AspNetCore.Routing.Template;
using Microsoft.OpenApi.Models;

namespace Vatsim.Scandinavia.FlightStrips.Host.Extensions;

public static partial class EndpointRouteBuilderExtensions
{
    public static RouteGroupBuilder MapAirportTenantRoutes(this IEndpointRouteBuilder builder,
        [StringSyntax("Route", typeof(RouteTemplate))] string prefix)
    {
        return builder.MapGroup("{airport:required}/" + prefix).WithOpenApi(x => x.AddAirportParameter()).AddEndpointFilter<AirportEndpointFilter>();
    }

    public static RouteGroupBuilder MapSessionTenantRoutes(this IEndpointRouteBuilder builder,
        [StringSyntax("Route", typeof(RouteTemplate))] string prefix)
    {
        return builder.MapGroup("{session:required}/" + prefix).WithOpenApi(x => x.AddSessionParameter());
    }

    public static RouteGroupBuilder MapAirportAndSessionTenantRoutes(this IEndpointRouteBuilder builder,
        [StringSyntax("Route", typeof(RouteTemplate))] string prefix)
    {
        return builder.MapGroup("{airport:required}/{session:required}/" + prefix)
            .WithOpenApi(x => x.AddAirportParameter().AddSessionParameter()).AddEndpointFilter<AirportEndpointFilter>();
    }

    private static OpenApiOperation AddAirportParameter(this OpenApiOperation operation)
    {
        operation.Parameters.Add(new OpenApiParameter
        {
            Description = "The airport ICAO code",
            Required = true,
            In = ParameterLocation.Path,
            Name = "airport",
            Schema = new OpenApiSchema {  Type = "string", MinLength = 4, MaxLength = 4, Pattern = "^[a-ZA-Z]{4}$"}
        });

        return operation;
    }

    private static OpenApiOperation AddSessionParameter(this OpenApiOperation operation)
    {
        operation.Parameters.Add(new OpenApiParameter
        {
            Description = "The session name",
            Required = true,
            In = ParameterLocation.Path,
            Name = "session",
            Schema = new OpenApiSchema {Type = "string", MinLength = 1}
        });

        return operation;
    }

    private partial class AirportEndpointFilter : IEndpointFilter
    {
        private static readonly Regex s_regex = AirportCode();

        public ValueTask<object?> InvokeAsync(EndpointFilterInvocationContext context, EndpointFilterDelegate next)
        {
            if (context.HttpContext.GetRouteData().Values["airport"] is string airport && s_regex.IsMatch(airport))
            {
                return next(context);
            }

            return ValueTask.FromResult<object?>(Results.ValidationProblem(new Dictionary<string, string[]>
            {
                {"airport", new[] {"The airport must be an airport ICAO code."}}
            }));
        }

        [GeneratedRegex("^[A-z]{4}$", RegexOptions.Compiled, 100)]
        private static partial Regex AirportCode();
    }
}
