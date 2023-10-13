using System.ComponentModel.DataAnnotations;

namespace Vatsim.Scandinavia.FlightStrips.Persistence.Redis;

public class RedisOptions
{
    public const string Redis = nameof(Redis);
    
    [Required]
    public string Host { get; set; } = string.Empty;

    public int Port { get; set; } = 6379;

    public string Password { get; set; } = string.Empty;


    public string GetConnectionString() => $"{Host}:{Port},password={Password}";
    
}