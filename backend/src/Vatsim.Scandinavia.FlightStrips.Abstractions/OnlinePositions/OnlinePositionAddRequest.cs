namespace Vatsim.Scandinavia.FlightStrips.Abstractions.OnlinePositions;

public record OnlinePositionAddRequest(OnlinePositionId Id, string Frequency, bool Plugin, bool Ui, string? DepartureRunway, string? ArrivalRunway);
