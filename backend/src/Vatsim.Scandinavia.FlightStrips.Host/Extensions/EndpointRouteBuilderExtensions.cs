using System.Diagnostics.CodeAnalysis;
using Microsoft.AspNetCore.Routing.Template;
using Microsoft.OpenApi.Models;

namespace Vatsim.Scandinavia.FlightStrips.Host.Extensions;

public static class EndpointRouteBuilderExtensions
{
    public static RouteGroupBuilder MapAirportTenantRoutes(this IEndpointRouteBuilder builder,
        [StringSyntax("Route", typeof(RouteTemplate))] string prefix)
    {
        return builder.MapGroup("{airport:length(4)}/" + prefix).WithOpenApi(x => x.AddAirportParameter());
    }

    public static RouteGroupBuilder MapSessionTenantRoutes(this IEndpointRouteBuilder builder,
        [StringSyntax("Route", typeof(RouteTemplate))] string prefix)
    {
        return builder.MapGroup("{session:required}/" + prefix).WithOpenApi(x => x.AddSessionParameter());
    }

    public static RouteGroupBuilder MapAirportAndSessionTenantRoutes(this IEndpointRouteBuilder builder,
        [StringSyntax("Route", typeof(RouteTemplate))] string prefix)
    {
        return builder.MapGroup("{airport:length(4)}/{session:required}/" + prefix)
            .WithOpenApi(x => x.AddAirportParameter().AddSessionParameter());
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
}
