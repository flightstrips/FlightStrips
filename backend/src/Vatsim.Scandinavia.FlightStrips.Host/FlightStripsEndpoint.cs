using FlightStrips;
using Grpc.Core;

namespace Vatsim.Scandinavia.FlightStrips.Host;

public class FlightStripsEndpoint(ILogger<FlightStripsEndpoint> logger) : FlightStripsService.FlightStripsServiceBase
{
    public override async Task Start(IAsyncStreamReader<ClientStreamMessage> requestStream,
        IServerStreamWriter<ServerStreamMessage> responseStream, ServerCallContext context)
    {
        await foreach (var message in requestStream.ReadAllAsync())
        {
#pragma warning disable CA1848
                logger.LogInformation("Got message {Message}", message.ToString());
#pragma warning restore CA1848
            if (message.MessageCase == ClientStreamMessage.MessageOneofCase.ClientInfo)
            {
                var serverMessage =
                    new ServerStreamMessage {SessionInfo = new SessionInfo {IsMaster = true, RelevantRange = 50}};
                await responseStream.WriteAsync(serverMessage);
            }
        }
    }
}
