using Microsoft.Extensions.Logging;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Strips;

namespace Vatsim.Scandinavia.FlightStrips;

public static partial class LogMessages
{
    [LoggerMessage(LogLevel.Information, "Setting sequence for {Strip} to {Sequence}")]
    public static partial void SetSequence(this ILogger logger, StripId strip, int? sequence);
}
