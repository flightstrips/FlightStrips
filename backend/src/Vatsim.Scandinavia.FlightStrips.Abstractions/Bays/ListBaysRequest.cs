namespace Vatsim.Scandinavia.FlightStrips.Abstractions.Bays;

public record ListBaysRequest(string Airport, bool? Default = null);