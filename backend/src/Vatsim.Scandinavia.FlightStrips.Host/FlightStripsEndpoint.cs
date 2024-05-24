using System.Diagnostics.CodeAnalysis;
using FlightStrips;
using Grpc.Core;

namespace Vatsim.Scandinavia.FlightStrips.Host;

[SuppressMessage("Performance", "CA1848:Use the LoggerMessage delegates")]
public sealed class FlightStripsEndpoint : FlightStripsService.FlightStripsServiceBase
{
    public override async Task Start(IAsyncStreamReader<ClientStreamMessage> requestStream,
        IServerStreamWriter<ServerStreamMessage> responseStream, ServerCallContext context)
    {
        using var handler = context.GetHttpContext().RequestServices.GetRequiredService<EuroScopeHandler>();
        await handler.StartAsync(requestStream, responseStream, context.CancellationToken);
    }
}
